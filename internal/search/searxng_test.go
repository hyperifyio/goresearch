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
