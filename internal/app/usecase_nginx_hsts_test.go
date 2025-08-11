package app

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// This end-to-end test exercises the use case described in
// .cursor/rules/go-usecase-nginx.mdc: Enable HSTS correctly on Nginx with
// includeSubDomains and preload. It validates report structure, references,
// evidence appendix, and manifest, using a stub LLM and local HTTP fixtures for
// deterministic behavior.
func TestUseCase_NginxHSTS_WithPreload_EndToEnd(t *testing.T) {
    t.Parallel()

    // Local HTTP fixtures approximating primary sources
    rfc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>RFC 6797</title></head><body><main><h1>HTTP Strict Transport Security (HSTS)</h1><p>Define Strict-Transport-Security header with max-age.</p></main></body></html>"))
    }))
    defer rfc.Close()
    nginx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>NGINX Docs</title></head><body><article><h1>Security headers</h1><p>add_header Strict-Transport-Security \"max-age=31536000; includeSubDomains; preload\" always;</p></article></body></html>"))
    }))
    defer nginx.Close()
    mdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>MDN HSTS</title></head><body><main><h1>HSTS</h1><p>Preload considerations and verification steps.</p></main></body></html>"))
    }))
    defer mdn.Close()
    // Bad source to verify per-source failure isolation (should be skipped)
    bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer bad.Close()

    // File-based search results to drive selection deterministically
    tmp := t.TempDir()
    resultsPath := filepath.Join(tmp, "results.json")
    type item struct{ Title, URL, Snippet string }
    items := []item{
        // Ensure snippets include fallback planner query substrings derived from the topic
        {Title: "RFC 6797", URL: rfc.URL, Snippet: "Enable HSTS correctly on Nginx (with preload) specification"},
        {Title: "NGINX HSTS Header", URL: nginx.URL, Snippet: "Enable HSTS correctly on Nginx (with preload) documentation"},
        {Title: "MDN HSTS", URL: mdn.URL, Snippet: "Enable HSTS correctly on Nginx (with preload) reference"},
        {Title: "Broken Source", URL: bad.URL, Snippet: "Enable HSTS correctly on Nginx (with preload) examples (bad)"},
    }
    if b, _ := json.Marshal(items); os.WriteFile(resultsPath, b, 0o644) != nil {
        t.Fatalf("write results")
    }

    // Brief input and output path
    briefPath := filepath.Join(tmp, "brief.md")
    // The topic mirrors the use case; content is small but enough to drive planner
    if err := os.WriteFile(briefPath, []byte("# Enable HSTS correctly on Nginx (with preload)\nAudience: engineers\nTone: concise\nTarget length: 400 words\n"), 0o644); err != nil {
        t.Fatalf("write brief: %v", err)
    }
    outPath := filepath.Join(tmp, "out.md")

    // Start stub LLM implementing planner/synth/verify
    const model = "test-model"
    llm := stubLLM(t, model)
    defer llm.Close()

    app, err := New(context.Background(), Config{
        InputPath:         briefPath,
        OutputPath:        outPath,
        FileSearchPath:    resultsPath,
        LLMModel:          model,
        LLMBaseURL:        llm.URL + "/v1",
        AllowPrivateHosts: true,
        CacheDir:          filepath.Join(tmp, "cache"),
        ReportsDir:        filepath.Join(tmp, "reports"),
    })
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()

    if err := app.Run(context.Background()); err != nil {
        t.Fatalf("run: %v", err)
    }

    // Basic output assertions per acceptance criteria
    b, err := os.ReadFile(outPath)
    if err != nil { t.Fatalf("read out: %v", err) }
    s := string(b)
    if !strings.Contains(s, "References") {
        t.Fatalf("missing References section")
    }
    if !strings.Contains(strings.ToLower(s), "evidence check") {
        t.Fatalf("missing Evidence check appendix")
    }
    if !strings.Contains(s, rfc.URL) || !strings.Contains(s, nginx.URL) {
        t.Fatalf("references should include RFC and NGINX docs; got:\n%s", s)
    }
    if !strings.Contains(s, "Reproducibility:") || !strings.Contains(strings.ToLower(s), "manifest") {
        t.Fatalf("missing reproducibility footer or manifest reference")
    }
}
