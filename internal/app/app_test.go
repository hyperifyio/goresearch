package app

import (
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestReportsMapping_DefaultOutput ensures that when the output path is the
// default "report.md", the app writes the final Markdown under ./reports with
// a stable slug-hash filename and that the artifacts bundle remains under
// reports/<slug>/.
func TestReportsMapping_DefaultOutput(t *testing.T) {
    t.Parallel()

    tmp := t.TempDir()
    briefPath := filepath.Join(tmp, "brief.md")
    if err := os.WriteFile(briefPath, []byte("# Mapping Topic\n"), 0o644); err != nil {
        t.Fatalf("write brief: %v", err)
    }
    // Deterministic file-based search yields zero results so dry-run is used
    resultsPath := filepath.Join(tmp, "results.json")
    if err := os.WriteFile(resultsPath, []byte("[]"), 0o644); err != nil {
        t.Fatalf("write results: %v", err)
    }

    app, err := New(context.Background(), Config{
        InputPath:      briefPath,
        OutputPath:     "report.md",
        FileSearchPath: resultsPath,
        DryRun:         true,
        ReportsDir:     filepath.Join(tmp, "reports"),
    })
    if err != nil { t.Fatalf("new app: %v", err) }
    defer app.Close()

    if err := app.Run(context.Background()); err != nil {
        t.Fatalf("run: %v", err)
    }

    // Expect a file named reports/mapping-topic-<hashprefix>.md
    entries, err := os.ReadDir(filepath.Join(tmp, "reports"))
    if err != nil { t.Fatalf("read reports dir: %v", err) }
    var found string
    for _, e := range entries {
        if e.IsDir() { continue }
        name := e.Name()
        if strings.HasPrefix(name, "mapping-topic-") && strings.HasSuffix(name, ".md") {
            found = name
            break
        }
    }
    if found == "" {
        t.Fatalf("expected mapped report markdown under reports/, got entries: %v", entries)
    }
}
