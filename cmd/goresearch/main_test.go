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

// Ensure --no-verify disables verification and --verify=false also disables.
func TestParseConfig_VerificationFlags(t *testing.T) {
    getenv := func(k string) string { return "" }
    cfg, _, err := parseConfig([]string{"-no-verify"}, getenv)
    if err != nil { t.Fatalf("parse: %v", err) }
    if !cfg.DisableVerify {
        t.Fatalf("-no-verify should set DisableVerify=true")
    }
    cfg, _, err = parseConfig([]string{"-verify=false"}, getenv)
    if err != nil { t.Fatalf("parse: %v", err) }
    if !cfg.DisableVerify {
        t.Fatalf("-verify=false should set DisableVerify=true")
    }
    cfg, _, err = parseConfig([]string{"-verify"}, getenv)
    if err != nil { t.Fatalf("parse: %v", err) }
    if cfg.DisableVerify {
        t.Fatalf("-verify (default true) should not disable")
    }
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

// Ensure single-file config is discovered and env vars override file values.
func TestConfigFile_DiscoveryAndEnvOverride(t *testing.T) {
    dir := t.TempDir()
    // Write a config file with model=A
    cfgPath := filepath.Join(dir, "goresearch.yaml")
    if err := os.WriteFile(cfgPath, []byte("llm:\n  model: A\ninput: i.md\noutput: o.md\n"), 0o644); err != nil {
        t.Fatalf("write cfg: %v", err)
    }
    // No flags; env sets model=B to override file, and provide required paths
    if err := os.WriteFile(filepath.Join(dir, "i.md"), []byte("# t"), 0o644); err != nil { t.Fatal(err) }
    oldWd, _ := os.Getwd()
    defer os.Chdir(oldWd)
    if err := os.Chdir(dir); err != nil { t.Fatal(err) }
    os.Setenv("LLM_MODEL", "B")
    defer os.Unsetenv("LLM_MODEL")
    // Parse with no args; parseConfig sees no flags; file loader will fill input/output
    cfg, _, err := parseConfig([]string{}, os.Getenv)
    if err != nil { t.Fatalf("parse: %v", err) }
    // Manually emulate main() sequencing for file discovery and overrides
    if fc, err := apppkg.LoadConfigFile("goresearch.yaml"); err == nil {
        apppkg.ApplyFileConfig(&cfg, fc)
    } else {
        t.Fatalf("load cfg: %v", err)
    }
    apppkg.ApplyEnvToConfig(&cfg)
    apppkg.ApplyEnvOverrides(&cfg)
    if got, want := cfg.LLMModel, "B"; got != want {
        t.Fatalf("env override failed: model=%q want %q", got, want)
    }
    if cfg.InputPath != "i.md" || cfg.OutputPath != "o.md" {
        t.Fatalf("file config not applied: %+v", cfg)
    }
}

// Ensure `goresearch init` scaffolds files idempotently.
func TestInitScaffold(t *testing.T) {
    dir := t.TempDir()
    if err := initScaffold(dir); err != nil { t.Fatalf("init: %v", err) }
    // Second call should not error and should keep files
    if err := initScaffold(dir); err != nil { t.Fatalf("init 2: %v", err) }
    if _, err := os.Stat(filepath.Join(dir, "goresearch.yaml")); err != nil { t.Fatalf("missing goresearch.yaml: %v", err) }
    if _, err := os.Stat(filepath.Join(dir, ".env.example")); err != nil { t.Fatalf("missing .env.example: %v", err) }
}

// Test for FEATURE_CHECKLIST item 239: CLI/options auto-generated reference
// Ensures the doc renderer includes key flags and environment variables.
func TestDoc_RenderIncludesKnownFlagsAndEnvs(t *testing.T) {
    fs := buildDocFlagSet(func(k string) string { return "" })
    md := renderCLIReferenceMarkdown(fs)
    mustContain := []string{
        "# goresearch CLI reference",
        "## Flags",
        "- `-input` (default: `request.md`)",
        "- `-output` (default: `report.md`)",
        "- `-llm.model`",
        "- `-searx.url`",
        "- `-cache.dir`",
        "- `-tools.enable`",
        "## Environment variables",
        "`LLM_BASE_URL`",
        "`SEARX_URL`",
        "`SOURCE_CAPS`",
    }
    for _, s := range mustContain {
        if !contains(md, s) {
            t.Fatalf("doc markdown missing %q\n---\n%s\n---", s, md)
        }
    }
}

// contains is a tiny helper to avoid strings import noise across tests.
func contains(haystack, needle string) bool { return len(haystack) >= len(needle) && (func() bool { return (stringIndex(haystack, needle) >= 0) })() }

// stringIndex provides strings.Index functionality without importing strings again here.
func stringIndex(s, substr string) int {
    // Simple O(n*m) scan is fine for short doc strings in tests
    n, m := len(s), len(substr)
    if m == 0 { return 0 }
    if m > n { return -1 }
    for i := 0; i <= n-m; i++ {
        if s[i:i+m] == substr { return i }
    }
    return -1
}
