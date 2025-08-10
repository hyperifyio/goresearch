package llmtools

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/hyperifyio/goresearch/internal/fetch"
    "github.com/hyperifyio/goresearch/internal/robots"
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
        SearchProvider: stubSearch{results: []search.Result{{Title: "Go", URL: "https://go.dev/?utm_source=x#frag", Snippet: "go site", Source: "stub"}}},
        FetchClient:    &fetch.Client{AllowPrivateHosts: true},
    }
    r, err := NewMinimalRegistry(deps)
    if err != nil { t.Fatalf("NewMinimalRegistry: %v", err) }

    // web_search
    def, ok := r.Get("web_search")
    if !ok { t.Fatalf("web_search not registered") }
    raw, err := def.Handler(context.Background(), mustRaw(t, map[string]any{"q":"golang","limit":5}))
    if err != nil { t.Fatalf("web_search handler: %v", err) }
    // After orchestration change, handler still returns raw "data"; envelope added by orchestrator.
    var out struct{ Results []struct{ Title, URL, Snippet, Source string } }
    if err := json.Unmarshal(raw, &out); err != nil { t.Fatalf("unmarshal: %v", err) }
    if len(out.Results) != 1 || out.Results[0].URL != "https://go.dev/" && out.Results[0].URL != "https://go.dev" {
        t.Fatalf("unexpected sanitized url: %+v", out.Results)
    }

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

func TestResultSizeBudgeting_TruncatesAndCachesBodiesAndExtracts(t *testing.T) {
    t.Parallel()
    // Serve a large HTML body
    large := strings.Repeat("A", 5000)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte(large))
    }))
    defer srv.Close()

    deps := MinimalDeps{
        SearchProvider: stubSearch{},
        FetchClient:    &fetch.Client{AllowPrivateHosts: true},
        MaxResultChars: 1000,
    }
    r, err := NewMinimalRegistry(deps)
    if err != nil { t.Fatalf("NewMinimalRegistry: %v", err) }

    // Execute fetch and verify truncation metadata
    fetchDef, ok := r.Get("fetch_url")
    if !ok { t.Fatalf("fetch_url not registered") }
    raw, err := fetchDef.Handler(context.Background(), mustRaw(t, map[string]any{"url": srv.URL}))
    if err != nil { t.Fatalf("fetch handler: %v", err) }
    var fetched struct{
        ContentType string `json:"content_type"`
        Body        string `json:"body"`
        Truncated   bool   `json:"truncated"`
        Bytes       int    `json:"bytes"`
        ID          string `json:"id"`
    }
    if err := json.Unmarshal(raw, &fetched); err != nil { t.Fatalf("unmarshal: %v", err) }
    if !fetched.Truncated || fetched.Bytes != 5000 || fetched.ID == "" || len(fetched.Body) != 1000 {
        t.Fatalf("unexpected truncation result: %+v", fetched)
    }

    // Retrieve full body via load_cached_body
    bodyDef, ok := r.Get("load_cached_body")
    if !ok { t.Fatalf("load_cached_body not registered") }
    raw, err = bodyDef.Handler(context.Background(), mustRaw(t, map[string]any{"id": fetched.ID}))
    if err != nil { t.Fatalf("load_cached_body: %v", err) }
    var full struct{ ContentType, Body string }
    if err := json.Unmarshal(raw, &full); err != nil { t.Fatalf("unmarshal: %v", err) }
    if full.Body != large {
        t.Fatalf("expected full body of length %d, got %d", len(large), len(full.Body))
    }

    // Test extract_main_text truncation path and recovery via load_cached_excerpt
    extractDef, _ := r.Get("extract_main_text")
    html := "<html><head><title>T</title></head><body><main><p>" + strings.Repeat("x", 6000) + "</p></main></body></html>"
    raw, err = extractDef.Handler(context.Background(), mustRaw(t, map[string]any{"html": html, "content_type":"text/html"}))
    if err != nil { t.Fatalf("extract handler: %v", err) }
    var doc struct{ ID, Title, Text string; Truncated bool; Bytes int }
    if err := json.Unmarshal(raw, &doc); err != nil { t.Fatalf("unmarshal: %v", err) }
    if !doc.Truncated || doc.Bytes <= 1000 || len(doc.Text) != 1000 {
        t.Fatalf("expected truncated extract, got %+v", doc)
    }
    // Load full excerpt
    loadDef, _ := r.Get("load_cached_excerpt")
    raw, err = loadDef.Handler(context.Background(), mustRaw(t, map[string]any{"id": doc.ID}))
    if err != nil { t.Fatalf("load_cached_excerpt: %v", err) }
    var fullDoc struct{ ID, Title, Text string }
    if err := json.Unmarshal(raw, &fullDoc); err != nil { t.Fatalf("unmarshal: %v", err) }
    if len(fullDoc.Text) <= 1000 || !strings.Contains(fullDoc.Text, strings.Repeat("x", 1000)) {
        t.Fatalf("expected full excerpt text, got length=%d", len(fullDoc.Text))
    }
}

func TestNewMinimalRegistry_MissingDeps(t *testing.T) {
    _, err := NewMinimalRegistry(MinimalDeps{})
    if err == nil { t.Fatalf("expected error when deps missing") }

    // Missing fetch client
    _, err = NewMinimalRegistry(MinimalDeps{SearchProvider: stubSearch{}})
    if err == nil { t.Fatalf("expected error for missing fetch client") }
}

// Verifies deny-on-disallow is enforced inside the fetch_url tool via the fetcher
// consulting robots.txt rules before network body retrieval.
// Requirement: FEATURE_CHECKLIST.md — Policy enforcement in tools (deny-on-disallow)
func TestFetchURL_DenyOnRobotsDisallow(t *testing.T) {
    // HTTP server that serves robots.txt disallowing /blocked and a simple HTML body otherwise
    mux := http.NewServeMux()
    mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain")
        _, _ = w.Write([]byte("User-agent: *\nDisallow: /blocked\n"))
    })
    mux.HandleFunc("/blocked", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<html><body>should not be fetched</body></html>"))
    })
    srv := httptest.NewServer(mux)
    defer srv.Close()

    // Robots manager used by fetch client to evaluate disallow before fetch
    rm := &robots.Manager{AllowPrivateHosts: true}
    client := &fetch.Client{
        AllowPrivateHosts: true,
        UserAgent:         "goresearch-test",
        Robots:            rm,
    }

    deps := MinimalDeps{SearchProvider: stubSearch{}, FetchClient: client}
    reg, err := NewMinimalRegistry(deps)
    if err != nil { t.Fatalf("NewMinimalRegistry: %v", err) }

    def, ok := reg.Get("fetch_url")
    if !ok { t.Fatalf("fetch_url not registered") }

    // Attempt to fetch a disallowed path
    _, err = def.Handler(context.Background(), mustRaw(t, map[string]any{"url": srv.URL + "/blocked"}))
    if err == nil {
        t.Fatalf("expected robots disallow error, got nil")
    }
    if reason, denied := fetch.IsRobotsDenied(err); !denied {
        t.Fatalf("expected robots denied error, got: %v", err)
    } else if reason == "" {
        t.Fatalf("robots denial should include a reason")
    }
}

// Verifies AI/TDM reuse opt-out via X-Robots-Tag is enforced inside tools.
// Requirement: FEATURE_CHECKLIST.md — Policy enforcement in tools (opt-out headers)
func TestFetchURL_ReuseDeniedByXRobotsTag(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Header().Set("X-Robots-Tag", "noai")
        _, _ = w.Write([]byte("<html><body>should be denied for reuse</body></html>"))
    }))
    defer srv.Close()

    client := &fetch.Client{AllowPrivateHosts: true, UserAgent: "goresearch-test"}
    deps := MinimalDeps{SearchProvider: stubSearch{}, FetchClient: client}
    reg, err := NewMinimalRegistry(deps)
    if err != nil { t.Fatalf("NewMinimalRegistry: %v", err) }
    def, ok := reg.Get("fetch_url")
    if !ok { t.Fatalf("fetch_url not registered") }

    _, err = def.Handler(context.Background(), mustRaw(t, map[string]any{"url": srv.URL}))
    if err == nil { t.Fatalf("expected reuse denied error, got nil") }
    if reason, denied := fetch.IsReuseDenied(err); !denied {
        t.Fatalf("expected reuse denied error, got: %v", err)
    } else if reason == "" {
        t.Fatalf("reuse denial should include a reason")
    }
}
