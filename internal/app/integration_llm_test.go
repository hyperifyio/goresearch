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

// stubLLM implements minimal OpenAI-compatible endpoints used by the app:
// - GET /v1/models: returns a list including the configured model
// - POST /v1/chat/completions: returns
//   * planner JSON when system message matches planner system
//   * synthesized Markdown when system matches synthesizer system
//   * verification JSON when system matches verifier system
func stubLLM(t *testing.T, model string) *httptest.Server {
    t.Helper()
    // Constants copied from packages to avoid import cycles
    plannerSystem := "You are a planning assistant. Respond with strict JSON only, no narration. The JSON schema is {\"queries\": string[6..10], \"outline\": string[5..8]}. Queries must be diverse and concise. Outline contains section headings only."
    synthSystem := "You are a careful technical writer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Keep style concise and factual."
    verifySystem := "You are a fact-check verifier. Respond with strict JSON only: {\"claims\":[{\"text\":string,\"citations\":int[],\"confidence\":\"high|medium|low\",\"supported\":bool}],\"summary\":string}. Extract 5-12 key factual claims. Map citations by numeric indices like [3]. If a claim lacks sufficient citation support, mark supported=false and set confidence=low."

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
            Model    string `json:"model"`
            Messages []struct {
                Role    string `json:"role"`
                Content string `json:"content"`
            } `json:"messages"`
        }
        _ = json.NewDecoder(r.Body).Decode(&req)
        // Expect exactly 2 messages: system then user
        sys := ""
        if len(req.Messages) > 0 {
            sys = strings.TrimSpace(req.Messages[0].Content)
        }
        var content string
        switch sys {
        case plannerSystem:
            // Return deterministic plan JSON (8 queries, 6 headings)
            plan := map[string]any{
                "queries": []string{
                    "Test Topic specification",
                    "Test Topic documentation",
                    "Test Topic reference",
                    "Test Topic tutorial",
                    "Test Topic best practices",
                    "Test Topic faq",
                    "Test Topic examples",
                    "Test Topic comparison",
                },
                "outline": []string{"Executive summary", "Background", "Core concepts", "Implementation guidance", "Examples", "Risks and limitations"},
            }
            b, _ := json.Marshal(plan)
            content = string(b)
        case synthSystem:
            // Produce minimal valid Markdown including references and inline citations
            // Extract URLs from the user message for determinism in assertions
            user := ""
            if len(req.Messages) >= 2 {
                user = req.Messages[1].Content
            }
            // crude parse: look for lines "n. title — url" and collect urls
            urls := make([]string, 0, 8)
            for _, line := range strings.Split(user, "\n") {
                line = strings.TrimSpace(line)
                if len(line) > 2 && line[0] >= '0' && line[0] <= '9' && strings.Contains(line, " — ") {
                    parts := strings.SplitN(line, " — ", 2)
                    if len(parts) == 2 {
                        url := strings.TrimSpace(parts[1])
                        urls = append(urls, url)
                    }
                }
            }
            // Use first two urls if present
            ref1, ref2 := "https://example.com/a", "https://example.com/b"
            if len(urls) >= 1 { ref1 = urls[0] }
            if len(urls) >= 2 { ref2 = urls[1] }
            content = "# Test Report\n2025-01-01\n\n## Executive summary\nA short summary citing [1].\n\n## Background\nBackground text with refs [1][2].\n\n## Risks and limitations\nSome cautions.\n\n## References\n1. Alpha — " + ref1 + "\n2. Beta — " + ref2 + "\n\n## Evidence check\nKey claims mapped."
        case verifySystem:
            // Return a minimal verification result JSON
            res := map[string]any{
                "claims": []map[string]any{
                    {"text": "Claim with [1]", "citations": []int{1}, "confidence": "medium", "supported": true},
                    {"text": "Another with [1][2]", "citations": []int{1, 2}, "confidence": "high", "supported": true},
                },
                "summary": "2 claims extracted; all supported.",
            }
            b, _ := json.Marshal(res)
            content = string(b)
        default:
            http.Error(w, "unexpected system", http.StatusBadRequest)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        resp := map[string]any{
            "choices": []map[string]any{
                {"message": map[string]string{"role": "assistant", "content": content}},
            },
        }
        _ = json.NewEncoder(w).Encode(resp)
    })
    return httptest.NewServer(mux)
}

// TestIntegration_FullPipeline_StubLLM validates the non-dry-run pipeline with
// a stub LLM and local HTTP fixtures. It asserts that references include the
// served URLs and that the evidence check appendix is appended.
func TestIntegration_FullPipeline_StubLLM(t *testing.T) {
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

    // Stub LLM implementing planner, synth, and verify contracts
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
    })
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()

    if err := app.Run(context.Background()); err != nil {
        t.Fatalf("run: %v", err)
    }
    out, err := os.ReadFile(outPath)
    if err != nil { t.Fatalf("read out: %v", err) }
    content := string(out)

    // Assertions: references include the served URLs and evidence section exists
    if !strings.Contains(content, "References") {
        t.Fatalf("missing References section")
    }
    if !strings.Contains(content, s1.URL) || !strings.Contains(content, s2.URL) {
        t.Fatalf("references missing fixture URLs\n---\n%s", content)
    }
    if !strings.Contains(content, "Evidence check") {
        t.Fatalf("missing Evidence check appendix")
    }
    // Basic inline citation presence
    if !strings.Contains(content, "[1]") {
        t.Fatalf("expected inline citation [1]")
    }
}
