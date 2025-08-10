package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	q.Set("language", "auto")
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
	return out, nil
}

type searxResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}
