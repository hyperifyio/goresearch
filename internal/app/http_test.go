package app

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/hyperifyio/goresearch/internal/search"
    "github.com/hyperifyio/goresearch/internal/extract"
)

func TestNewHighThroughputHTTPClient_Config(t *testing.T) {
	c := newHighThroughputHTTPClient()
	if c.Timeout == 0 {
		t.Fatalf("expected non-zero timeout")
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected http.Transport")
	}
	if tr.MaxIdleConnsPerHost < 100 {
		t.Fatalf("expected large MaxIdleConnsPerHost, got %d", tr.MaxIdleConnsPerHost)
	}
	// Ensure we didn't return the default client's transport
	if reflect.ValueOf(http.DefaultTransport).Pointer() == reflect.ValueOf(tr).Pointer() {
		t.Fatalf("transport should not be default")
	}
}

type fakeGetter struct {
	fail   map[string]error
	bodies map[string][]byte
}

func (f fakeGetter) get(ctx context.Context, url string) ([]byte, string, error) {
	if err, ok := f.fail[url]; ok {
		return nil, "", err
	}
	if b, ok := f.bodies[url]; ok {
		return b, "text/html", nil
	}
	return nil, "", http.ErrMissingFile
}

func TestFetchAndExtract_PerSourceIsolation(t *testing.T) {
	// Two URLs where the first fails and the second succeeds. We expect the
	// function to skip the first and still return one excerpt for the second.
	selected := []struct{ title, url string }{
		{"Bad", "https://bad.example"},
		{"Good", "https://good.example"},
	}
	// Minimal HTML body for the good URL
	goodHTML := []byte("<!doctype html><html><head><title>Good</title></head><body><main><p>Hello</p></main></body></html>")
	fg := fakeGetter{
		fail:   map[string]error{"https://bad.example": http.ErrHandlerTimeout},
		bodies: map[string][]byte{"https://good.example": goodHTML},
	}
	// Convert to search.Results for input
	var results []search.Result
	for _, s := range selected {
		results = append(results, search.Result{Title: s.title, URL: s.url})
	}
    out, skipped := fetchAndExtract(context.Background(), fg, extract.HeuristicExtractor{}, results, Config{PerSourceChars: 1000, AllowPrivateHosts: true})
    if len(out) != 1 || len(skipped) != 0 {
        t.Fatalf("expected 1 excerpt and 0 skipped (non-robots failure), got excerpts=%d skipped=%d", len(out), len(skipped))
	}
	if out[0].Index != 1 {
		t.Fatalf("expected index 1, got %d", out[0].Index)
	}
	if out[0].URL != "https://good.example" {
		t.Fatalf("expected good URL, got %s", out[0].URL)
	}
	if out[0].Title == "" {
		t.Fatalf("expected non-empty title")
	}
	if out[0].Excerpt == "" {
		t.Fatalf("expected non-empty excerpt")
	}
}
