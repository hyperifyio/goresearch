package app

import (
    "context"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "testing"
)

// TestGolden_FullPipeline compares the generated Markdown report against a
// golden file, normalizing timestamps, digests, ports, and other benign
// differences. This enforces stable output formatting and detects regressions.
// Requirement: FEATURE_CHECKLIST.md — Golden output comparisons
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestGolden_FullPipeline(t *testing.T) {
    t.Parallel()

    // Local HTML fixtures
    s1 := newTestHTMLServer("Alpha", "<main><h1>A</h1><p>Alpha body</p></main>")
    defer s1.Close()
    s2 := newTestHTMLServer("Beta", "<article><h1>B</h1><p>Beta body</p></article>")
    defer s2.Close()

    tmp := t.TempDir()
    briefPath := filepath.Join(tmp, "brief.md")
    if err := os.WriteFile(briefPath, []byte("# Test Topic\nAudience: engineers\nTone: concise\nTarget length: 200 words\n"), 0o644); err != nil {
        t.Fatalf("write brief: %v", err)
    }
    outPath := filepath.Join(tmp, "out.md")

    // Provide deterministic file-based search results that include our fixture URLs
    resultsPath := filepath.Join(tmp, "results.json")
    data := "[\n  {\"Title\":\"Alpha Page\",\"URL\":\"" + s1.URL + "\",\"Snippet\":\"Test Topic specification\"},\n  {\"Title\":\"Beta Page\",\"URL\":\"" + s2.URL + "\",\"Snippet\":\"Test Topic documentation\"}\n]"
    if err := os.WriteFile(resultsPath, []byte(data), 0o644); err != nil {
        t.Fatalf("write results: %v", err)
    }

    // Stub LLM for planner/synth/verify
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

    raw, err := os.ReadFile(outPath)
    if err != nil { t.Fatalf("read out: %v", err) }
    got := normalizeGolden(string(raw), s1.URL, s2.URL)

    goldenPath := filepath.Join("testdata", "golden_full_report.md")
    if os.Getenv("UPDATE_GOLDEN") == "1" {
        if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil { t.Fatalf("mkdir: %v", err) }
        if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
            t.Fatalf("update golden: %v", err)
        }
    }
    wantBytes, err := os.ReadFile(goldenPath)
    if err != nil { t.Fatalf("read golden: %v", err) }
    want := string(wantBytes)

    if strings.TrimSpace(got) != strings.TrimSpace(want) {
        t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
    }
}

// normalizeGolden replaces environment-specific or time-varying content with
// stable placeholders so the golden comparison focuses on semantic structure.
func normalizeGolden(in string, alphaURL, betaURL string) string {
    s := in
    // Normalize dynamic URLs
    s = strings.ReplaceAll(s, alphaURL, "ALPHA_URL")
    s = strings.ReplaceAll(s, betaURL, "BETA_URL")
    // Normalize volatile access dates appended by references enrichment
    s = regexp.MustCompile(`(?i)\s*\(Accessed on \d{4}-\d{2}-\d{2}\)`).ReplaceAllString(s, "")
    // Normalize LLM base URL in both footer and manifest
    s = regexp.MustCompile(`llm_base_url=[^;\n]+`).ReplaceAllString(s, "llm_base_url=LLM_BASE_URL")
    s = regexp.MustCompile(`LLM base URL:.*`).ReplaceAllString(s, "LLM base URL: LLM_BASE_URL")
    // Normalize Generated timestamp line in manifest
    s = regexp.MustCompile(`Generated: .+`).ReplaceAllString(s, "Generated: 2000-01-01T00:00:00Z")
    // Normalize SHA256 digests and character counts in manifest entries
    s = regexp.MustCompile(`sha256=[a-f0-9]{64}`).ReplaceAllString(s, "sha256=SHA256")
    s = regexp.MustCompile(`chars=\d+`).ReplaceAllString(s, "chars=CHARS")
    // Trim trailing spaces and collapse CRLF
    s = strings.ReplaceAll(s, "\r\n", "\n")
    lines := strings.Split(s, "\n")
    for i := range lines {
        lines[i] = strings.TrimRight(lines[i], " \t")
    }
    return strings.Join(lines, "\n")
}

// newTestHTMLServer is a tiny helper to serve deterministic HTML content.
func newTestHTMLServer(title string, bodyInnerHTML string) *httptest.Server {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte("<!doctype html><html><head><title>" + title + "</title></head><body>" + bodyInnerHTML + "</body></html>"))
    })
    return httptest.NewServer(handler)
}
