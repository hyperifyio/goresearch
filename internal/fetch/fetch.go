package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hyperifyio/goresearch/internal/cache"
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

	contentType := resp.Header.Get("Content-Type")
	if !isAllowedHTMLContentType(contentType) {
		return nil, "", "", "", resp.StatusCode, fmt.Errorf("unsupported content type: %s", contentType)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", "", resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return b, contentType, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), resp.StatusCode, nil
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
