package main

import (
	"os"
	"path/filepath"
	"testing"

	apppkg "github.com/hyperifyio/goresearch/internal/app"
    "github.com/hyperifyio/goresearch/internal/synth"
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

// Ensures exit code policy conditions are surfaced as errors from run().
func TestRun_NoUsableSources_Error(t *testing.T) {
    // Configure with no search provider and empty selected results will lead
    // to zero excerpts; the app should return ErrNoUsableSources.
    dir := t.TempDir()
    in := filepath.Join(dir, "in.md")
    out := filepath.Join(dir, "out.md")
    if err := os.WriteFile(in, []byte("# Topic\n"), 0o644); err != nil {
        t.Fatalf("write input: %v", err)
    }
    cfg := apppkg.Config{
        InputPath:  in,
        OutputPath: out,
        CacheDir:   filepath.Join(dir, "cache"),
        // No searx URL -> no provider -> no selected sources
        LLMModel:   "dummy", // prevent empty model causing early error
    }
    err := run(cfg)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if err != apppkg.ErrNoUsableSources {
        t.Fatalf("expected ErrNoUsableSources, got %v", err)
    }
}

// When synthesizer returns no substantive body, ensure error is propagated so
// the CLI can map it to a nonzero exit.
func TestIsNoSubstantiveBody(t *testing.T) {
    if !isNoSubstantiveBody(synth.ErrNoSubstantiveBody) {
        t.Fatalf("expected true for ErrNoSubstantiveBody")
    }
}
