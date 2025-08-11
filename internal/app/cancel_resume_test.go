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
    "time"
)

// delayedLLM is a minimal OpenAI-compatible stub that inserts delays so we can
// trigger context cancellation during a run. It supports /v1/models and
// /v1/chat/completions for planner and synthesizer paths.
func delayedLLM(t *testing.T, model string, synthDelay time.Duration) *httptest.Server {
    t.Helper()
    plannerSystem := "You are a planning assistant. Respond with strict JSON only, no narration. The JSON schema is {\"queries\": string[6..10], \"outline\": string[5..8]}. Queries must be diverse and concise. Outline contains section headings only."
    synthSystem := "You are a careful technical writer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Keep style concise and factual."

    mux := http.NewServeMux()
    mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{
            "data": []map[string]any{{"id": model, "object": "model"}},
        })
    })
    mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
        defer r.Body.Close()
        var req struct {
            Messages []struct{ Role, Content string } `json:"messages"`
        }
        _ = json.NewDecoder(r.Body).Decode(&req)
        sys := ""
        if len(req.Messages) > 0 {
            sys = strings.TrimSpace(req.Messages[0].Content)
        }
        var content string
        switch sys {
        case plannerSystem:
            plan := map[string]any{
                "queries": []string{"A spec", "A docs", "A ref", "A tut", "A best", "A faq", "A ex", "A lim"},
                "outline": []string{"Executive summary", "Background", "Core", "Guidance", "Alternatives & conflicting evidence", "Examples", "Risks and limitations"},
            }
            b, _ := json.Marshal(plan)
            content = string(b)
        case synthSystem:
            if synthDelay > 0 {
                time.Sleep(synthDelay)
            }
            content = "# Test Report\n2025-01-01\n\n## Executive summary\nA short summary.\n\n## References\n1. Alpha — https://example.com/alpha\n"
        default:
            http.Error(w, "unexpected system", http.StatusBadRequest)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{
            "choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
        })
    })
    return httptest.NewServer(mux)
}

// TestGracefulCancelAndResume ensures that when the context is canceled during
// the run, partial artifacts are written and a subsequent run succeeds using
// the same cache and inputs.
func TestGracefulCancelAndResume(t *testing.T) {
    t.Parallel()

    // Slow HTML fixtures to give time window for cancellation
    mkServer := func(title, body string, delay time.Duration) *httptest.Server {
        return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            time.Sleep(delay)
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            _, _ = w.Write([]byte("<!doctype html><html><head><title>" + title + "</title></head><body><main><p>" + body + "</p></main></body></html>"))
        }))
    }
    s1 := mkServer("Alpha", "Alpha body", 400*time.Millisecond)
    defer s1.Close()
    s2 := mkServer("Beta", "Beta body", 600*time.Millisecond)
    defer s2.Close()

    tmp := t.TempDir()
    // File-based search results ensure deterministic selection order
    resultsPath := filepath.Join(tmp, "results.json")
    type item struct{ Title, URL, Snippet string }
    items := []item{{Title: "Alpha Page", URL: s1.URL, Snippet: "A spec"}, {Title: "Beta Page", URL: s2.URL, Snippet: "A docs"}}
    b, _ := json.Marshal(items)
    if err := os.WriteFile(resultsPath, b, 0o644); err != nil { t.Fatalf("write results: %v", err) }

    briefPath := filepath.Join(tmp, "brief.md")
    if err := os.WriteFile(briefPath, []byte("# A Topic\nAudience: engineers\n"), 0o644); err != nil { t.Fatalf("write brief: %v", err) }
    outPath := filepath.Join(tmp, "out.md")

    // Delayed LLM so synthesis doesn't finish before we cancel
    const model = "test-model"
    llm := delayedLLM(t, model, 2*time.Second)
    defer llm.Close()

    cfg := Config{
        InputPath:         briefPath,
        OutputPath:        outPath,
        FileSearchPath:    resultsPath,
        LLMModel:          model,
        LLMBaseURL:        llm.URL + "/v1",
        AllowPrivateHosts: true,
        CacheDir:          filepath.Join(tmp, "cache"),
        ReportsDir:        filepath.Join(tmp, "reports"),
    }

    // Start run with a context that cancels shortly after extraction begins
    ctx, cancel := context.WithCancel(context.Background())
    go func() {
        time.Sleep(500 * time.Millisecond)
        cancel()
    }()

    app, err := New(ctx, cfg)
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()
    if err := app.Run(ctx); err == nil {
        t.Fatalf("expected cancellation error, got nil")
    }

    // Verify partial artifacts written (planner and selection at minimum)
    bundleDir := filepath.Join(tmp, "reports", "a-topic")
    if _, err := os.Stat(filepath.Join(bundleDir, "planner.json")); err != nil {
        t.Fatalf("expected planner.json written on cancel: %v", err)
    }
    if _, err := os.Stat(filepath.Join(bundleDir, "selected.json")); err != nil {
        t.Fatalf("expected selected.json written on cancel: %v", err)
    }
    // extracts.json may or may not exist depending on timing; do not assert

    // Second run without cancellation should complete successfully, reusing cache where possible
    app2, err := New(context.Background(), cfg)
    if err != nil { t.Fatalf("new app2: %v", err) }
    defer app2.Close()
    if err := app2.Run(context.Background()); err != nil {
        t.Fatalf("second run failed: %v", err)
    }
    out, err := os.ReadFile(outPath)
    if err != nil { t.Fatalf("read out: %v", err) }
    if !strings.Contains(string(out), "References") {
        t.Fatalf("second run output missing References section")
    }
}
