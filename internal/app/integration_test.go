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

// TestIntegration_DryRun_FileProvider exercises planner fallback, file-search provider,
// selection, and budgeting in dry-run mode, writing a deterministic report.
func TestIntegration_DryRun_FileProvider(t *testing.T) {
    t.Parallel()

    // Set up two local HTML pages; not fetched in dry-run but included as selected URLs.
    s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>Alpha</title></head><body>Alpha</body></html>"))
    }))
    defer s1.Close()
    s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>Beta</title></head><body>Beta</body></html>"))
    }))
    defer s2.Close()

    // File-based search results used by dry-run path
    tmp := t.TempDir()
    resultsPath := filepath.Join(tmp, "results.json")
    type item struct{ Title, URL, Snippet string }
    // Snippets include substrings that match fallback planner queries so they are selected
    // e.g., queries like "Test Topic specification" and "Test Topic documentation"
    items := []item{
        {Title: "Alpha Page", URL: s1.URL, Snippet: "Test Topic specification alpha"},
        {Title: "Beta Page", URL: s2.URL, Snippet: "Test Topic documentation beta"},
    }
    b, _ := json.Marshal(items)
    if err := os.WriteFile(resultsPath, b, 0o644); err != nil { t.Fatalf("write results: %v", err) }

    // Brief and output paths
    briefPath := filepath.Join(tmp, "brief.md")
    if err := os.WriteFile(briefPath, []byte("# Test Topic\nAudience: engineers\nTone: concise\nTarget length: 200 words\n"), 0o644); err != nil { t.Fatalf("write brief: %v", err) }
    outPath := filepath.Join(tmp, "out.md")

    // Stub LLM base URL to avoid external network during New(); respond 404 fast
    llmSrv := httptest.NewServer(http.NotFoundHandler())
    defer llmSrv.Close()

    app, err := New(context.Background(), Config{
        InputPath:      briefPath,
        OutputPath:     outPath,
        FileSearchPath: resultsPath,
        // Ensure fallback planner runs by leaving model empty
        LLMModel:   "",
        LLMBaseURL: llmSrv.URL,
        DryRun:     true,
    })
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()

    if err := app.Run(context.Background()); err != nil {
        t.Fatalf("run: %v", err)
    }
    out, err := os.ReadFile(outPath)
    if err != nil { t.Fatalf("read out: %v", err) }
    if len(out) == 0 { t.Fatalf("empty output") }
    content := string(out)
    if !strings.Contains(content, "Planned queries:") { t.Fatalf("missing planned queries") }
    if !strings.Contains(content, s1.URL) || !strings.Contains(content, s2.URL) { t.Fatalf("missing selected URLs") }
}