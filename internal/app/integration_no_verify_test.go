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

// TestIntegration_SkipVerification_WhenDisabled validates that when DisableVerify
// is set, the pipeline completes but does not append the Evidence check appendix.
func TestIntegration_SkipVerification_WhenDisabled(t *testing.T) {
    t.Parallel()

    // HTML fixtures
    s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>Alpha</title></head><body><main><h1>A</h1><p>Alpha body</p></main></body></html>"))
    }))
    defer s1.Close()
    s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>Beta</title></head><body><article><h1>B</h1><p>Beta body</p></article></body></html>"))
    }))
    defer s2.Close()

    // File-based search results to ensure deterministic selection
    tmp := t.TempDir()
    resultsPath := filepath.Join(tmp, "results.json")
    type item struct{ Title, URL, Snippet string }
    items := []item{
        {Title: "Alpha Page", URL: s1.URL, Snippet: "Test Topic specification"},
        {Title: "Beta Page", URL: s2.URL, Snippet: "Test Topic documentation"},
    }
    b, _ := json.Marshal(items)
    if err := os.WriteFile(resultsPath, b, 0o644); err != nil { t.Fatalf("write results: %v", err) }

    // Brief and output paths
    briefPath := filepath.Join(tmp, "brief.md")
    if err := os.WriteFile(briefPath, []byte("# Test Topic\nAudience: engineers\nTone: concise\nTarget length: 200 words\n"), 0o644); err != nil { t.Fatalf("write brief: %v", err) }
    outPath := filepath.Join(tmp, "out.md")

    // Stub LLM implementing planner/synth/verify; verify path should be skipped
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
        DisableVerify:     true,
    })
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()

    if err := app.Run(context.Background()); err != nil {
        t.Fatalf("run: %v", err)
    }
    out, err := os.ReadFile(outPath)
    if err != nil { t.Fatalf("read out: %v", err) }
    content := string(out)

    // Must include references but not the appended Evidence appendix details
    if !strings.Contains(content, "References") {
        t.Fatalf("missing References section")
    }
    // Our appended evidence appendix includes structured lines with "— cites [..]; confidence: ..".
    if strings.Contains(content, "; confidence:") || strings.Contains(content, " — cites ") {
        t.Fatalf("expected no appended Evidence appendix details when verification disabled\n---\n%s", content)
    }
}
