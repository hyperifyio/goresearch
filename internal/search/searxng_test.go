package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearxNG_Search_ParsesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Doc", "url": "https://example.com", "content": "snippet"},
				{"title": "Bad", "url": "", "content": "no url"},
			},
		})
	}))
	defer srv.Close()

	s := &SearxNG{BaseURL: srv.URL, HTTPClient: srv.Client()}
	got, err := s.Search(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 valid result, got %d", len(got))
	}
	if got[0].URL != "https://example.com" {
		t.Fatalf("unexpected url: %q", got[0].URL)
	}
}

func TestSearxNG_Search_DomainPolicyFilters(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{
            "results": []map[string]any{
                {"title": "A", "url": "https://a.example.com/x", "content": "a"},
                {"title": "B", "url": "https://b.test.org/y", "content": "b"},
            },
        })
    }))
    defer srv.Close()

    s := &SearxNG{BaseURL: srv.URL, HTTPClient: srv.Client(), Policy: DomainPolicy{Allowlist: []string{"example.com"}}}
    got, err := s.Search(context.Background(), "q", 10)
    if err != nil { t.Fatalf("search: %v", err) }
    if len(got) != 1 || got[0].URL != "https://a.example.com/x" { t.Fatalf("unexpected filtered results: %+v", got) }

    s2 := &SearxNG{BaseURL: srv.URL, HTTPClient: srv.Client(), Policy: DomainPolicy{Denylist: []string{"example.com"}}}
    got2, err := s2.Search(context.Background(), "q", 10)
    if err != nil { t.Fatalf("search: %v", err) }
    if len(got2) != 1 || got2[0].URL != "https://b.test.org/y" { t.Fatalf("unexpected deny filtered results: %+v", got2) }
}
