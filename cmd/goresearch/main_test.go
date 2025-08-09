package main

import (
	"os"
	"path/filepath"
	"testing"

	apppkg "github.com/hyperifyio/goresearch/internal/app"
)

// Smoke test: ensure main.run writes output in dry-run mode with minimal config.
func TestRun_DryRun_WritesOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	out := filepath.Join(dir, "out.md")
	if err := os.WriteFile(in, []byte("# Test Topic\nAudience: devs\nTone: terse"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	cfg := apppkg.Config{
		InputPath:  in,
		OutputPath: out,
		DryRun:     true,
		CacheDir:   filepath.Join(dir, "cache"),
	}
	if err := run(cfg); err != nil {
		t.Fatalf("run error: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil || len(b) == 0 {
		t.Fatalf("expected output file, err=%v", err)
	}
}
