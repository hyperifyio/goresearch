package llmtools

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/hyperifyio/goresearch/internal/fetch"
    "github.com/hyperifyio/goresearch/internal/search"
)

type stubSearch struct{ results []search.Result; err error }
func (s stubSearch) Name() string { return "stub" }
func (s stubSearch) Search(ctx context.Context, q string, limit int) ([]search.Result, error) {
    if s.err != nil { return nil, s.err }
    if limit > len(s.results) { limit = len(s.results) }
    return s.results[:limit], nil
}

type stubFetch struct{ body string; ct string; err error }

func (s *stubFetch) Get(ctx context.Context, url string) ([]byte, string, error) {
    if s.err != nil { return nil, "", s.err }
    return []byte(s.body), s.ct, nil
}

func TestNewMinimalRegistry_WebSearchAndExtractFlow(t *testing.T) {
    deps := MinimalDeps{
        SearchProvider: stubSearch{results: []search.Result{{Title: "Go", URL: "https://go.dev", Snippet: "go site", Source: "stub"}}},
        FetchClient:    &fetch.Client{AllowPrivateHosts: true},
    }
    r, err := NewMinimalRegistry(deps)
    if err != nil { t.Fatalf("NewMinimalRegistry: %v", err) }

    // web_search
    def, ok := r.Get("web_search")
    if !ok { t.Fatalf("web_search not registered") }
    raw, err := def.Handler(context.Background(), mustRaw(t, map[string]any{"q":"golang","limit":5}))
    if err != nil { t.Fatalf("web_search handler: %v", err) }
    var out struct{ Results []struct{ Title, URL, Snippet, Source string } }
    if err := json.Unmarshal(raw, &out); err != nil { t.Fatalf("unmarshal: %v", err) }
    if len(out.Results) != 1 || out.Results[0].URL != "https://go.dev" { t.Fatalf("unexpected results: %+v", out.Results) }

    // extract_main_text
    def, ok = r.Get("extract_main_text")
    if !ok { t.Fatalf("extract_main_text not registered") }
    html := "<html><head><title>A</title></head><body><main><h1>Hdr</h1><p>x</p></main></body></html>"
    raw, err = def.Handler(context.Background(), mustRaw(t, map[string]any{"html": html, "content_type":"text/html"}))
    if err != nil { t.Fatalf("extract handler: %v", err) }
    var doc struct{ ID, Title, Text string }
    if err := json.Unmarshal(raw, &doc); err != nil { t.Fatalf("unmarshal: %v", err) }
    if doc.ID == "" || doc.Title != "A" || doc.Text == "" { t.Fatalf("bad doc: %+v", doc) }

    // load_cached_excerpt
    def, ok = r.Get("load_cached_excerpt")
    if !ok { t.Fatalf("load_cached_excerpt not registered") }
    raw, err = def.Handler(context.Background(), mustRaw(t, map[string]any{"id": doc.ID}))
    if err != nil { t.Fatalf("load_cached_excerpt: %v", err) }
    var loaded struct{ ID, Title, Text string }
    if err := json.Unmarshal(raw, &loaded); err != nil { t.Fatalf("unmarshal: %v", err) }
    if loaded.ID != doc.ID || loaded.Text == "" { t.Fatalf("mismatch: %+v vs %+v", loaded, doc) }
}

func TestNewMinimalRegistry_FetchURL_ErrorSurface(t *testing.T) {
    deps := MinimalDeps{
        SearchProvider: stubSearch{},
        FetchClient:    &fetch.Client{AllowPrivateHosts: true},
    }
    r, err := NewMinimalRegistry(deps)
    if err != nil { t.Fatalf("NewMinimalRegistry: %v", err) }
    def, _ := r.Get("fetch_url")
    // Use a client with unsupported scheme error via URL
    _, err = def.Handler(context.Background(), mustRaw(t, map[string]any{"url": "file:///etc/hosts"}))
    if err == nil { t.Fatalf("expected error for unsupported scheme") }
}

func TestNewMinimalRegistry_MissingDeps(t *testing.T) {
    _, err := NewMinimalRegistry(MinimalDeps{})
    if err == nil { t.Fatalf("expected error when deps missing") }

    // Missing fetch client
    _, err = NewMinimalRegistry(MinimalDeps{SearchProvider: stubSearch{}})
    if err == nil { t.Fatalf("expected error for missing fetch client") }
}
