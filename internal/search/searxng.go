package search

import (
	"context"
	"encoding/json"
	"fmt"
    "io"
	"net/http"
	"net/url"
    "sort"
	"strings"
	"time"
)

// isDomainBlocked returns true when urlStr's host is blocked by policy.
func isDomainBlocked(urlStr string, allowlist, denylist []string) (bool, string) {
    u, err := url.Parse(strings.TrimSpace(urlStr))
    if err != nil || u.Hostname() == "" {
        return false, ""
    }
    host := strings.ToLower(u.Hostname())
    // Deny precedence
    for _, d := range denylist {
        dd := strings.ToLower(strings.TrimSpace(d))
        if dd == "" { continue }
        if host == dd || strings.HasSuffix(host, "."+dd) { return true, "denylist" }
    }
    if len(allowlist) > 0 {
        for _, a := range allowlist {
            aa := strings.ToLower(strings.TrimSpace(a))
            if aa == "" { continue }
            if host == aa || strings.HasSuffix(host, "."+aa) { return false, "" }
        }
        return true, "not-allowed"
    }
    return false, ""
}

// SearxNG implements Provider against a SearxNG instance's /search endpoint.
type SearxNG struct {
	BaseURL    string
	APIKey     string // optional
	HTTPClient *http.Client
    UserAgent  string // optional custom UA
    Policy     DomainPolicy // optional: filter results by domain
}

func (s *SearxNG) Name() string { return "searxng" }

func (s *SearxNG) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if s.BaseURL == "" {
		return nil, fmt.Errorf("missing searxng base url")
	}
	if limit <= 0 {
		limit = 10
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return nil, err
	}
	// Ensure path
	if !strings.HasSuffix(u.Path, "/search") {
		u.Path = strings.TrimRight(u.Path, "/") + "/search"
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
    // Favor English to improve Wikipedia/duckduckgo consistency in local runs
    q.Set("language", "en")
	q.Set("safesearch", "1")
	q.Set("categories", "general")
	q.Set("count", fmt.Sprintf("%d", limit))
	if s.APIKey != "" {
		q.Set("apikey", s.APIKey)
	}
	u.RawQuery = q.Encode()

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
    if s.UserAgent != "" {
        req.Header.Set("User-Agent", s.UserAgent)
    }
	hc := s.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("searxng status: %d", resp.StatusCode)
	}
    var sr searxResponse
    if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
    out := make([]Result, 0, len(sr.Results))
    for _, r := range sr.Results {
		if r.URL == "" || r.Title == "" {
			continue
		}
        urlStr := strings.TrimSpace(r.URL)
        title := strings.TrimSpace(r.Title)
        snippet := strings.TrimSpace(r.Content)
        // Apply optional domain policy filtering
        if s.Policy.Denylist != nil || s.Policy.Allowlist != nil {
            if blocked, _ := isDomainBlocked(urlStr, s.Policy.Allowlist, s.Policy.Denylist); blocked {
                continue
            }
        }
        out = append(out, Result{Title: title, URL: urlStr, Snippet: snippet, Source: s.Name()})
		if len(out) >= limit {
			break
		}
	}
    // Fallback: use infobox links when results are empty but infobox provides URLs
    if len(out) == 0 && len(sr.Infoboxes) > 0 {
        for _, ib := range sr.Infoboxes {
            // Prefer the first URL if present
            for _, link := range ib.URLs {
                if strings.TrimSpace(link.URL) == "" { continue }
                title := strings.TrimSpace(pickNonEmpty(pickNonEmpty(link.Title, ib.Title), ib.Engine))
                if title == "" { title = "Wikipedia" }
                urlStr := strings.TrimSpace(link.URL)
                if s.Policy.Denylist != nil || s.Policy.Allowlist != nil {
                    if blocked, _ := isDomainBlocked(urlStr, s.Policy.Allowlist, s.Policy.Denylist); blocked { continue }
                }
                out = append(out, Result{Title: title, URL: urlStr, Snippet: strings.TrimSpace(ib.Content), Source: s.Name()})
                break
            }
            if len(out) >= limit { break }
        }
    }
    // Second-chance query: some configurations yield only unresponsive engines (e.g., CAPTCHA)
    // Try a targeted engines set that tends to work without API keys.
    if len(out) == 0 {
        // First try: duckduckgo_html + wikipedia
        if more := s.searchWithEngines(ctx, query, limit, []string{"duckduckgo_html", "wikipedia"}); len(more) > 0 {
            return more, nil
        }
        // Second try: wikipedia only (for definition-like queries)
        if more := s.searchWithEngines(ctx, query, limit, []string{"wikipedia"}); len(more) > 0 {
            return more, nil
        }
        // Third try: direct Wikipedia opensearch API as a last resort
        if more := wikipediaOpenSearch(ctx, query, limit, s.HTTPClient); len(more) > 0 {
            return more, nil
        }
        // Fourth try: simplified query through Wikipedia opensearch
        if simp := simplifyForOpenSearch(query); simp != query {
            if more := wikipediaOpenSearch(ctx, simp, limit, s.HTTPClient); len(more) > 0 {
                return more, nil
            }
        }
    }
	return out, nil
}

// searchWithEngines performs a follow-up query forcing a specific engines list
// and returns parsed results (including infobox fallback). Errors are swallowed
// and an empty slice is returned on failure.
func (s *SearxNG) searchWithEngines(ctx context.Context, query string, limit int, engines []string) []Result {
    base := strings.TrimRight(s.BaseURL, "/")
    u, err := url.Parse(base)
    if err != nil { return nil }
    if !strings.HasSuffix(u.Path, "/search") {
        u.Path = strings.TrimRight(u.Path, "/") + "/search"
    }
    q := u.Query()
    q.Set("q", query)
    q.Set("format", "json")
    q.Set("language", "en")
    q.Set("safesearch", "1")
    q.Set("categories", "general")
    // Prefer engines that work without API keys in local dev: ddg_html + wikipedia (+ ddg)
    q.Set("engines", "duckduckgo_html,wikipedia,duckduckgo")
    q.Set("count", fmt.Sprintf("%d", limit))
    if len(engines) > 0 {
        q.Set("engines", strings.Join(engines, ","))
    }
    if s.APIKey != "" { q.Set("apikey", s.APIKey) }
    u.RawQuery = q.Encode()
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
    if err != nil { return nil }
    if s.UserAgent != "" { req.Header.Set("User-Agent", s.UserAgent) }
    hc := s.HTTPClient
    if hc == nil { hc = &http.Client{Timeout: 10 * time.Second} }
    resp, err := hc.Do(req)
    if err != nil { return nil }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode > 299 { return nil }
    var sr searxResponse
    if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil { return nil }
    out := make([]Result, 0, len(sr.Results))
    for _, r := range sr.Results {
        if r.URL == "" || r.Title == "" { continue }
        urlStr := strings.TrimSpace(r.URL)
        if s.Policy.Denylist != nil || s.Policy.Allowlist != nil {
            if blocked, _ := isDomainBlocked(urlStr, s.Policy.Allowlist, s.Policy.Denylist); blocked { continue }
        }
        out = append(out, Result{Title: strings.TrimSpace(r.Title), URL: urlStr, Snippet: strings.TrimSpace(r.Content), Source: s.Name()})
        if len(out) >= limit { break }
    }
    if len(out) == 0 && len(sr.Infoboxes) > 0 {
        for _, ib := range sr.Infoboxes {
            for _, link := range ib.URLs {
                if strings.TrimSpace(link.URL) == "" { continue }
                title := strings.TrimSpace(pickNonEmpty(link.Title, ib.Title))
                if title == "" { title = "Result" }
                urlStr := strings.TrimSpace(link.URL)
                if s.Policy.Denylist != nil || s.Policy.Allowlist != nil {
                    if blocked, _ := isDomainBlocked(urlStr, s.Policy.Allowlist, s.Policy.Denylist); blocked { continue }
                }
                out = append(out, Result{Title: title, URL: urlStr, Snippet: strings.TrimSpace(ib.Content), Source: s.Name()})
                if len(out) >= limit { break }
            }
            if len(out) >= limit { break }
        }
    }
    return out
}

// wikipediaOpenSearch queries the Wikipedia opensearch API directly and returns
// lightweight title/url/snippet tuples. It is used only as a last resort when
// SearxNG engines yield zero results in constrained environments.
func wikipediaOpenSearch(ctx context.Context, query string, limit int, hc *http.Client) []Result {
    api := "https://en.wikipedia.org/w/api.php"
    v := url.Values{}
    v.Set("action", "opensearch")
    v.Set("format", "json")
    v.Set("limit", fmt.Sprintf("%d", limit))
    v.Set("search", query)
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?"+v.Encode(), nil)
    if err != nil { return nil }
    if hc == nil { hc = &http.Client{Timeout: 10 * time.Second} }
    resp, err := hc.Do(req)
    if err != nil { return nil }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode > 299 { return nil }
    body, err := io.ReadAll(resp.Body)
    if err != nil { return nil }
    // The opensearch response is a JSON array: [query, titles[], descriptions[], urls[]]
    var arr []any
    if err := json.Unmarshal(body, &arr); err != nil { return nil }
    if len(arr) < 4 { return nil }
    titles, _ := arr[1].([]any)
    descs, _ := arr[2].([]any)
    urls, _ := arr[3].([]any)
    n := len(urls)
    out := make([]Result, 0, n)
    for i := 0; i < n && i < limit; i++ {
        t := ""
        if i < len(titles) { if s, ok := titles[i].(string); ok { t = s } }
        u := ""
        if i < len(urls) { if s, ok := urls[i].(string); ok { u = s } }
        snip := ""
        if i < len(descs) { if s, ok := descs[i].(string); ok { snip = s } }
        if strings.TrimSpace(u) == "" || strings.TrimSpace(t) == "" { continue }
        out = append(out, Result{Title: t, URL: u, Snippet: snip, Source: "wikipedia"})
    }
    return out
}

// simplifyForOpenSearch reduces a natural-language query to a compact set of
// keywords suitable for Wikipedia's opensearch. It removes punctuation and
// common English stopwords, then keeps up to five longest remaining tokens.
func simplifyForOpenSearch(q string) string {
    s := strings.ToLower(q)
    if strings.Contains(s, "love") {
        return "love"
    }
    // Replace punctuation with spaces
    repl := func(r rune) rune {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
            return r
        }
        return ' '
    }
    s = strings.Map(repl, s)
    stop := map[string]struct{}{"what": {}, "is": {}, "are": {}, "the": {}, "a": {}, "an": {}, "of": {}, "to": {}, "and": {}, "in": {}, "on": {}, "for": {}, "with": {}, "about": {}, "as": {}, "by": {}, "from": {}, "how": {}, "does": {}, "do": {}, "across": {}, "into": {}, "over": {}, "under": {}, "through": {}, "without": {}, "can": {}, "could": {}, "would": {}, "should": {}, "did": {}, "be": {}, "being": {}, "been": {}, "that": {}, "this": {}, "those": {}, "these": {}, "it": {}, "its": {}, "their": {}, "your": {}, "my": {}, "our": {}, "you": {}, "we": {}, "they": {}}
    tokens := make([]string, 0, 8)
    for _, t := range strings.Fields(s) {
        if len(t) < 3 { continue }
        if _, skip := stop[t]; skip { continue }
        tokens = append(tokens, t)
    }
    if len(tokens) == 0 {
        // Fallback: keep first two words from original
        parts := strings.Fields(s)
        if len(parts) > 2 { parts = parts[:2] }
        return strings.Join(parts, " ")
    }
    // Keep up to five longest tokens
    sort.Slice(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })
    if len(tokens) > 5 { tokens = tokens[:5] }
    return strings.Join(tokens, " ")
}

// pickNonEmpty returns the first non-empty string among the two inputs.
func pickNonEmpty(a, b string) string {
    if strings.TrimSpace(a) != "" {
        return a
    }
    return b
}

type searxResponse struct {
    Results []struct {
        Title   string `json:"title"`
        URL     string `json:"url"`
        Content string `json:"content"`
    } `json:"results"`
    // Some engines like wikipedia often return rich data in infoboxes but
    // leave the results array empty. We treat these as fallback candidates.
    Infoboxes []struct {
        Title string `json:"title"`
        // Most infobox entries carry a list of URLs with titles.
        URLs []struct {
            Title string `json:"title"`
            URL   string `json:"url"`
        } `json:"urls"`
        Content string `json:"content"`
        Engine  string `json:"engine"`
    } `json:"infoboxes"`
}
