package app

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "testing"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

// TestOperationalLogs_Stages ensures that an end-to-end run emits
// structured logs for each major pipeline stage so the run is
// deterministic and auditable.
// Requirement: FEATURE_CHECKLIST.md — Operational run clarity
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestOperationalLogs_Stages(t *testing.T) {
    t.Parallel()

    // Capture logs to a buffer
    var buf bytes.Buffer
    oldLogger := log.Logger
    log.Logger = zerolog.New(&buf).With().Timestamp().Logger()
    t.Cleanup(func() { log.Logger = oldLogger })

    // Minimal deterministic inputs: file-based search and stub LLM
    tmp := t.TempDir()
    briefPath := filepath.Join(tmp, "brief.md")
    if err := os.WriteFile(briefPath, []byte("# Topic\n"), 0o644); err != nil {
        t.Fatalf("write brief: %v", err)
    }
    resultsPath := filepath.Join(tmp, "results.json")
    if err := os.WriteFile(resultsPath, []byte("[]"), 0o644); err != nil {
        t.Fatalf("write results: %v", err)
    }
    outPath := filepath.Join(tmp, "out.md")

    // Use dry-run mode to avoid network/LLM while exercising early stages
    app, err := New(context.Background(), Config{
        InputPath:      briefPath,
        OutputPath:     outPath,
        FileSearchPath: resultsPath,
        DryRun:         true,
    })
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()

    if err := app.Run(context.Background()); err != nil {
        t.Fatalf("run: %v", err)
    }

    logs := buf.String()
    mustContain := []string{
        `"stage":"brief"`,
        `"stage":"planner"`,
        `"stage":"selection"`,
    }
    for _, needle := range mustContain {
        if !strings.Contains(logs, needle) {
            t.Fatalf("expected log to contain %s; got logs:\n%s", needle, logs)
        }
    }

    // Verify elapsed duration fields appear as numbers
    if !regexp.MustCompile(`"elapsed":\d+`).MatchString(logs) {
        t.Fatalf("expected elapsed field in logs; got:\n%s", logs)
    }
}
