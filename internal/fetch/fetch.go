package fetch

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net"
    "net/url"
    "strings"
    "sync"
    "time"

    "github.com/hyperifyio/goresearch/internal/cache"
    "github.com/hyperifyio/goresearch/internal/robots"
    "golang.org/x/net/html"
)

// Client wraps http.Client and provides timeouts and limited retry on transient errors.
type Client struct {
	HTTPClient *http.Client
	UserAgent  string
	// MaxAttempts includes the initial attempt. Minimum 1.
	MaxAttempts int
	// PerRequestTimeout bounds each request.
	PerRequestTimeout time.Duration
	// Optional on-disk cache for HTTP GET bodies and headers.
	Cache *cache.HTTPCache
	// If true, bypass cache entirely and fetch fresh (no conditional headers),
	// but still save the latest response to cache.
	BypassCache bool

	// RedirectMaxHops caps redirect following to avoid loops. Zero means default (5).
	RedirectMaxHops int
	// MaxConcurrent limits concurrent in-flight requests per client instance.
	// Zero means unlimited.
	MaxConcurrent int

	// internal limiter initialized on first use when MaxConcurrent > 0
	limiter     chan struct{}
	limiterOnce sync.Once

    // AllowPrivateHosts, when true, disables the "public web only" guard that
    // rejects localhost and private IP ranges. Intended for tests only.
    AllowPrivateHosts bool

    // EnablePDF, when true, allows fetching and accepting application/pdf bodies.
    // The caller is responsible for choosing an appropriate extractor.
    EnablePDF bool

    // Robots, when provided, enables crawl-delay compliance based on robots.txt
    // rules. The client will schedule requests per host to respect any declared
    // Crawl-delay for the most specific matching User-agent group.
    Robots *robots.Manager

    // internal per-host scheduler state for crawl-delay
    crawlMu      sync.Mutex
    nextAllowed  map[string]time.Time
}

// ReuseDeniedError is returned when a response contains headers that opt out of
// text-and-data-mining reuse (e.g., X-Robots-Tag: noai, notrain). Callers
// should treat this as a hard denial and skip using the content.
type ReuseDeniedError struct{
    Reason string
}

func (e ReuseDeniedError) Error() string { return "reuse denied: " + e.Reason }

// IsReuseDenied reports whether the error indicates reuse denial and returns an
// explanatory reason when true.
func IsReuseDenied(err error) (string, bool) {
    var r ReuseDeniedError
    if errors.As(err, &r) {
        return r.Reason, true
    }
    return "", false
}

func (c *Client) getHTTPClient() *http.Client {
	if c.HTTPClient != nil {
		// Clone to attach our redirect policy without mutating caller's client
		base := *c.HTTPClient
		base.CheckRedirect = c.checkRedirectFunc()
		return &base
	}
	return &http.Client{Timeout: c.PerRequestTimeout, CheckRedirect: c.checkRedirectFunc()}
}

// Get issues a GET with context, user-agent, and bounded retry for transient errors.
func (c *Client) Get(ctx context.Context, url string) ([]byte, string, error) {
	// If cache exists, attempt conditional request
	var etag, lastMod string
	if c.Cache != nil && !c.BypassCache {
		if meta, err := c.Cache.LoadMeta(ctx, url); err == nil && meta != nil {
			etag = meta.ETag
			lastMod = meta.LastModified
		}
	}
	attempts := c.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		body, ct, newEtag, newLastMod, status, err := c.tryOnce(ctx, url, etag, lastMod)
		if err == nil {
			// Save/serve from cache
			if c.Cache != nil && status == 200 {
				_ = c.Cache.Save(ctx, url, ct, newEtag, newLastMod, body)
			}
			// If 304 and cache available, return cached body
			if status == 304 && c.Cache != nil {
				if cached, err := c.Cache.LoadBody(ctx, url); err == nil {
					return cached, ct, nil
				}
			}
			return body, ct, nil
		}
		if !isTransient(err) || i == attempts-1 {
			return nil, "", err
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("unknown error")
	}
	return nil, "", lastErr
}

func (c *Client) tryOnce(ctx context.Context, url string, etag string, lastMod string) ([]byte, string, string, string, int, error) {
    // Concurrency gate per client instance
    c.acquire()
    defer c.release()

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", "", "", 0, fmt.Errorf("new request: %w", err)
	}
	// Reject non-HTTP(S) schemes early
	if req.URL == nil || !isHTTPScheme(req.URL) {
		return nil, "", "", "", 0, fmt.Errorf("unsupported URL scheme: %q", req.URL.String())
	}
    // Reject embedded credentials to avoid authenticating to private services
    if req.URL.User != nil {
        return nil, "", "", "", 0, errors.New("credentials in URL unsupported")
    }
    // Public web only: reject localhost and private/link-local addresses by literal IP or known names
    host := req.URL.Hostname()
    if !c.AllowPrivateHosts && isLocalOrPrivateHost(host) {
        return nil, "", "", "", 0, fmt.Errorf("private host not allowed: %s", host)
    }

    // Respect per-host Crawl-delay if robots manager is configured
    if c.Robots != nil {
        if err := c.awaitCrawlDelay(ctx, req.URL); err != nil {
            return nil, "", "", "", 0, err
        }
    }
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	httpClient := c.getHTTPClient()
	if c.PerRequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(req.Context(), c.PerRequestTimeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", "", "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		return nil, "", "", "", resp.StatusCode, fmt.Errorf("server error: %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotModified {
		// 304: no body expected; return no error with status 304
		return nil, resp.Header.Get("Content-Type"), resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), resp.StatusCode, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", "", "", resp.StatusCode, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

    // Honor X-Robots-Tag opt-out directives for AI/TDM reuse
    if reason, denied := detectTDMOptOut(resp.Header, c.UserAgent); denied {
        return nil, "", "", "", resp.StatusCode, ReuseDeniedError{Reason: reason}
    }
    // Honor TDM reservation via HTTP Link headers (in or around content)
    if reason, denied := detectTDMReservationLinkHeader(resp.Header); denied {
        return nil, "", "", "", resp.StatusCode, ReuseDeniedError{Reason: reason}
    }

    contentType := resp.Header.Get("Content-Type")
    if !(isAllowedHTMLContentType(contentType) || (c.EnablePDF && isAllowedPDFContentType(contentType))) {
		return nil, "", "", "", resp.StatusCode, fmt.Errorf("unsupported content type: %s", contentType)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", "", resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
    // For HTML/XHTML, also honor page-level meta robots/googlebot opt-out
    if isAllowedHTMLContentType(contentType) {
        if reason, denied := detectMetaTDMOptOut(b); denied {
            return nil, "", "", "", resp.StatusCode, ReuseDeniedError{Reason: reason}
        }
        // And honor <link rel="tdm-reservation"> in the document head
        if reason, denied := detectHTMLTDMReservationLink(b); denied {
            return nil, "", "", "", resp.StatusCode, ReuseDeniedError{Reason: reason}
        }
    }
	return b, contentType, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), resp.StatusCode, nil
}

// detectTDMOptOut checks X-Robots-Tag style headers for AI/TDM opt-out signals.
// Returns a human-readable reason and true when reuse is denied.
func detectTDMOptOut(h http.Header, userAgent string) (string, bool) {
    if h == nil { return "", false }
    // Collect all X-Robots-Tag values (case-insensitive per HTTP semantics)
    vals := h.Values("X-Robots-Tag")
    if len(vals) == 0 {
        return "", false
    }
    // Join values and split on commas/semicolons; accept tokens optionally
    // scoped like "googlebot: noai" — we conservatively deny on any noai/notrain.
    joined := strings.ToLower(strings.Join(vals, ","))
    // Fast-path containment check
    if !strings.Contains(joined, "noai") && !strings.Contains(joined, "notrain") {
        return "", false
    }
    // Tokenize
    tokens := splitTokens(joined)
    for _, t := range tokens {
        tt := strings.TrimSpace(t)
        switch tt {
        case "noai":
            return "X-Robots-Tag:noai", true
        case "notrain":
            return "X-Robots-Tag:notrain", true
        default:
            // also support scoped forms like "googlebot: noai"
            if strings.Contains(tt, ":") {
                parts := strings.SplitN(tt, ":", 2)
                if len(parts) == 2 {
                    dir := strings.TrimSpace(parts[1])
                    if dir == "noai" || dir == "notrain" {
                        return "X-Robots-Tag:" + dir, true
                    }
                }
            }
        }
    }
    return "", false
}

func splitTokens(s string) []string {
    // Split by comma and semicolon
    parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        if pp := strings.TrimSpace(p); pp != "" { out = append(out, pp) }
    }
    return out
}

// detectMetaTDMOptOut scans HTML for <meta name="robots" ...> and
// <meta name="googlebot" ...> directives that include noai/notrain.
// Returns a human-readable reason and true when reuse is denied.
func detectMetaTDMOptOut(body []byte) (string, bool) {
    if len(body) == 0 {
        return "", false
    }
    z := html.NewTokenizer(bytes.NewReader(body))
    inHead := false
    for {
        tt := z.Next()
        switch tt {
        case html.ErrorToken:
            return "", false
        case html.StartTagToken, html.SelfClosingTagToken:
            tn, _ := z.TagName()
            tagName := strings.ToLower(string(tn))
            if tagName == "head" {
                inHead = true
                continue
            }
            if tagName == "body" && inHead {
                // Stop once we reach body; meta directives live in head
                return "", false
            }
            if tagName != "meta" {
                continue
            }
            var nameVal, contentVal string
            // Extract attributes (case-insensitive names)
            for {
                key, val, more := z.TagAttr()
                k := strings.ToLower(string(key))
                v := string(val)
                if k == "name" || k == "http-equiv" {
                    if nameVal == "" {
                        nameVal = strings.ToLower(strings.TrimSpace(v))
                    }
                } else if k == "content" {
                    contentVal = v
                }
                if !more {
                    break
                }
            }
            if nameVal == "robots" || nameVal == "googlebot" || nameVal == "x-robots-tag" {
                if contentVal == "" {
                    continue
                }
                joined := strings.ToLower(contentVal)
                if !strings.Contains(joined, "noai") && !strings.Contains(joined, "notrain") {
                    continue
                }
                // Tokenize by comma/semicolon and optional scoped tokens like "googlebot: noai"
                tokens := splitTokens(joined)
                for _, t := range tokens {
                    ttok := strings.TrimSpace(t)
                    if ttok == "noai" {
                        scope := nameVal
                        if scope == "x-robots-tag" { scope = "meta x-robots-tag" } else { scope = "meta "+scope }
                        return scope+":noai", true
                    }
                    if ttok == "notrain" {
                        scope := nameVal
                        if scope == "x-robots-tag" { scope = "meta x-robots-tag" } else { scope = "meta "+scope }
                        return scope+":notrain", true
                    }
                    if strings.Contains(ttok, ":") {
                        parts := strings.SplitN(ttok, ":", 2)
                        dir := strings.TrimSpace(parts[1])
                        if dir == "noai" || dir == "notrain" {
                            scope := nameVal
                            if scope == "x-robots-tag" { scope = "meta x-robots-tag" } else { scope = "meta "+scope }
                            return scope+":"+dir, true
                        }
                    }
                }
            }
        }
    }
}

// detectTDMReservationLinkHeader inspects HTTP Link headers for a parameter
// rel="tdm-reservation" (case-insensitive). Presence indicates a reservation of
// rights for text-and-data-mining; we conservatively deny reuse.
func detectTDMReservationLinkHeader(h http.Header) (string, bool) {
    if h == nil { return "", false }
    vals := h.Values("Link")
    if len(vals) == 0 { return "", false }
    for _, v := range vals {
        // The Link header can contain multiple entries separated by commas, but
        // commas may also appear inside quoted strings. For a conservative check,
        // perform a case-insensitive search for rel=tdm-reservation in the value.
        vv := strings.ToLower(v)
        if strings.Contains(vv, "rel=\"tdm-reservation\"") || strings.Contains(vv, "rel='tdm-reservation'") || strings.Contains(vv, "rel=tdm-reservation") {
            return "Link: rel=tdm-reservation", true
        }
    }
    return "", false
}

// detectHTMLTDMReservationLink scans the HTML head for <link rel="tdm-reservation" ...>
// Presence indicates a reservation of rights for TDM; deny reuse.
func detectHTMLTDMReservationLink(body []byte) (string, bool) {
    if len(body) == 0 { return "", false }
    z := html.NewTokenizer(bytes.NewReader(body))
    inHead := false
    for {
        tt := z.Next()
        switch tt {
        case html.ErrorToken:
            return "", false
        case html.StartTagToken, html.SelfClosingTagToken:
            tn, _ := z.TagName()
            tagName := strings.ToLower(string(tn))
            if tagName == "head" { inHead = true; continue }
            if tagName == "body" && inHead { return "", false }
            if tagName != "link" { continue }
            var relVal string
            for {
                key, val, more := z.TagAttr()
                k := strings.ToLower(string(key))
                v := strings.ToLower(strings.TrimSpace(string(val)))
                if k == "rel" { relVal = v }
                if !more { break }
            }
            if relVal == "" { continue }
            // rel attribute can contain space-separated tokens
            tokens := strings.Fields(relVal)
            for _, t := range tokens {
                if t == "tdm-reservation" {
                    return "link rel=tdm-reservation", true
                }
            }
        }
    }
}

// awaitCrawlDelay enforces per-host crawl-delay based on robots.txt rules.
// It schedules request start times so that consecutive requests to the same
// host are spaced by at least the configured delay. If no delay is set for
// the effective user agent, it returns immediately.
func (c *Client) awaitCrawlDelay(ctx context.Context, u *url.URL) error {
    if u == nil || c.Robots == nil {
        return nil
    }
    scheme := strings.ToLower(u.Scheme)
    host := u.Hostname()
    hostWithPort := u.Host // includes port when present
    if scheme == "" || hostWithPort == "" {
        return nil
    }
    robotsURL := scheme + "://" + hostWithPort + "/robots.txt"
    // Fetch and cache rules (memory/disk cache inside Manager keeps this cheap)
    rules, _, err := c.Robots.Get(ctx, robotsURL)
    if err != nil {
        // On robots errors, be conservative and proceed without delay rather than
        // blocking fetches, since separate checklist items govern 404/5xx policy.
        return nil
    }
    d := rules.CrawlDelayFor(c.UserAgent)
    if d == nil || *d <= 0 {
        return nil
    }
    delay := *d

    // Reserve a start time slot for this host and sleep until then (context-aware)
    now := time.Now()
    c.crawlMu.Lock()
    if c.nextAllowed == nil { c.nextAllowed = make(map[string]time.Time) }
    startAt := now
    if next, ok := c.nextAllowed[host]; ok && next.After(now) {
        startAt = next
    }
    c.nextAllowed[host] = startAt.Add(delay)
    c.crawlMu.Unlock()

    if startAt.After(now) {
        wait := time.Until(startAt)
        if wait > 0 {
            timer := time.NewTimer(wait)
            defer timer.Stop()
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-timer.C:
            }
        }
    }
    return nil
}

func isTransient(err error) bool {
	// Treat HTTP 5xx and context deadline as transient.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// crude check for server error text
	return contains(err.Error(), "server error:")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	// simple substring search to avoid importing strings for small surface
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (c *Client) checkRedirectFunc() func(req *http.Request, via []*http.Request) error {
	max := c.RedirectMaxHops
	if max <= 0 {
		max = 5
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= max {
			return errors.New("too many redirects")
		}
		// Only allow http/https during redirects
		if req.URL == nil || !isHTTPScheme(req.URL) {
			return errors.New("redirect to unsupported scheme")
		}
		return nil
	}
}

func isHTTPScheme(u *url.URL) bool {
	if u == nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

func isAllowedHTMLContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	// allow text/html variants and application/xhtml+xml
	return strings.HasPrefix(ct, "text/html") || strings.HasPrefix(ct, "application/xhtml+xml")
}

// isAllowedPDFContentType returns true for PDF content types.
func isAllowedPDFContentType(ct string) bool {
    ct = strings.ToLower(strings.TrimSpace(ct))
    return strings.HasPrefix(ct, "application/pdf")
}

// isLocalOrPrivateHost returns true for localhost names and literal IPs that are
// loopback, private, or link-local. Hostname resolution is intentionally not
// performed here to keep this check deterministic and side-effect free.
func isLocalOrPrivateHost(host string) bool {
    h := strings.ToLower(strings.TrimSpace(host))
    if h == "localhost" || h == "localhost.localdomain" || h == "::1" || h == "[::1]" {
        return true
    }
    if ip := net.ParseIP(h); ip != nil {
        if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
            return true
        }
    }
    return false
}

func (c *Client) acquire() {
	if c.MaxConcurrent <= 0 {
		return
	}
	c.limiterOnce.Do(func() {
		c.limiter = make(chan struct{}, c.MaxConcurrent)
	})
	c.limiter <- struct{}{}
}

func (c *Client) release() {
	if c.MaxConcurrent <= 0 || c.limiter == nil {
		return
	}
	select {
	case <-c.limiter:
	default:
		// should not happen, but avoid blocking
	}
}
