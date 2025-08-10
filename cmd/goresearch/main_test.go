package main

import (
    "os"
    "path/filepath"
    "reflect"
    "testing"
    "time"

    apppkg "github.com/hyperifyio/goresearch/internal/app"
    "github.com/hyperifyio/goresearch/internal/synth"
)

// Test parsing defaults and environment fallbacks
func TestParseConfig_DefaultsAndEnv(t *testing.T) {
    env := map[string]string{
        "SEARX_URL":       "https://searx.example.com",
        "SEARX_KEY":       "k",
        "LLM_BASE_URL":    "http://llm.local",
        "LLM_MODEL":       "gpt-oss",
        "LLM_API_KEY":     "abc",
        "TOPIC_HASH":      "deadbeef",
    }
    getenv := func(k string) string { return env[k] }
    args := []string{"-input", "in.md", "-output", "out.md"}
    cfg, verbose, err := parseConfig(args, getenv)
    if err != nil { t.Fatalf("parseConfig error: %v", err) }
    if verbose { t.Fatalf("verbose should default to false") }
    if cfg.InputPath != "in.md" || cfg.OutputPath != "out.md" {
        t.Fatalf("paths mismatch: %+v", cfg)
    }
    if cfg.SearxURL != env["SEARX_URL"] || cfg.SearxKey != env["SEARX_KEY"] {
        t.Fatalf("searx env not applied: %+v", cfg)
    }
    if cfg.LLMBaseURL != env["LLM_BASE_URL"] || cfg.LLMModel != env["LLM_MODEL"] || cfg.LLMAPIKey != env["LLM_API_KEY"] {
        t.Fatalf("llm env not applied: %+v", cfg)
    }
    // Tools defaults
    if cfg.ToolsEnabled { t.Fatalf("ToolsEnabled default false") }
    if cfg.ToolsDryRun { t.Fatalf("ToolsDryRun default false") }
    if cfg.ToolsMaxCalls != 32 { t.Fatalf("ToolsMaxCalls default 32, got %d", cfg.ToolsMaxCalls) }
    if cfg.ToolsPerToolTimeout != 10*time.Second { t.Fatalf("ToolsPerToolTimeout default 10s, got %s", cfg.ToolsPerToolTimeout) }
    if cfg.ToolsMode != "harmony" { t.Fatalf("ToolsMode default harmony, got %q", cfg.ToolsMode) }
}

// Test flags override for tools orchestration and prompt file override
func TestParseConfig_ToolsFlagsAndPromptFile(t *testing.T) {
    dir := t.TempDir()
    synthFile := filepath.Join(dir, "synth.txt")
    if err := os.WriteFile(synthFile, []byte("SYS_PROMPT"), 0o644); err != nil {
        t.Fatalf("write: %v", err)
    }
    env := map[string]string{}
    getenv := func(k string) string { return env[k] }
    args := []string{
        "-tools.enable",
        "-tools.dryRun",
        "-tools.maxCalls", "5",
        "-tools.maxWallClock", "2s",
        "-tools.perToolTimeout", "50ms",
        "-tools.mode", "legacy",
        "-synth.systemPromptFile", synthFile,
        "-robots.overrideDomains", "example.com, docs.local",
        "-robots.overrideConfirm",
        "-domains.allow", "a.com,b.com",
        "-domains.deny", "c.com",
    }
    cfg, _, err := parseConfig(args, getenv)
    if err != nil { t.Fatalf("parseConfig error: %v", err) }
    if !cfg.ToolsEnabled || !cfg.ToolsDryRun { t.Fatalf("tools enable/dryRun not set: %+v", cfg) }
    if cfg.ToolsMaxCalls != 5 { t.Fatalf("ToolsMaxCalls=5, got %d", cfg.ToolsMaxCalls) }
    if cfg.ToolsMaxWallClock != 2*time.Second { t.Fatalf("ToolsMaxWallClock=2s, got %s", cfg.ToolsMaxWallClock) }
    if cfg.ToolsPerToolTimeout != 50*time.Millisecond { t.Fatalf("ToolsPerToolTimeout=50ms, got %s", cfg.ToolsPerToolTimeout) }
    if cfg.ToolsMode != "legacy" { t.Fatalf("ToolsMode=legacy, got %q", cfg.ToolsMode) }
    if cfg.SynthSystemPrompt != "SYS_PROMPT" { t.Fatalf("synth prompt file not loaded: %q", cfg.SynthSystemPrompt) }
    if !cfg.RobotsOverrideConfirm { t.Fatalf("robots override confirm not set") }
    if got, want := cfg.RobotsOverrideAllowlist, []string{"example.com", "docs.local"}; !reflect.DeepEqual(got, want) {
        t.Fatalf("robots allowlist = %v, want %v", got, want)
    }
    if got, want := cfg.DomainAllowlist, []string{"a.com","b.com"}; !reflect.DeepEqual(got, want) {
        t.Fatalf("allowlist = %v, want %v", got, want)
    }
    if got, want := cfg.DomainDenylist, []string{"c.com"}; !reflect.DeepEqual(got, want) {
        t.Fatalf("denylist = %v, want %v", got, want)
    }
}

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
