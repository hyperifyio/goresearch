package app

import (
    "os"
    "path/filepath"
    "testing"
)

// Intent: Implements FEATURE_CHECKLIST.md item "Environment & secrets handling — .env support".
// This test verifies that LoadEnvFiles reads KEY=VALUE pairs and populates os.Environ.
func TestLoadEnvFiles_LoadsKeyValues(t *testing.T) {
    t.Setenv("FOO", "")
    t.Setenv("BAR", "")

    dir := t.TempDir()
    envPath := filepath.Join(dir, ".env.test")
    content := "\n# sample dotenv file\nFOO=alpha\nBAR=beta\n"
    if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
        t.Fatalf("write dotenv: %v", err)
    }

    if err := LoadEnvFiles(envPath); err != nil {
        t.Fatalf("LoadEnvFiles error: %v", err)
    }

    if got := os.Getenv("FOO"); got != "alpha" {
        t.Fatalf("FOO=%q, want alpha", got)
    }
    if got := os.Getenv("BAR"); got != "beta" {
        t.Fatalf("BAR=%q, want beta", got)
    }
}

// Later files override earlier ones when loading multiple dotenv files.
func TestLoadEnvFiles_OverrideOrder(t *testing.T) {
    t.Setenv("K", "")
    dir := t.TempDir()
    a := filepath.Join(dir, ".env.a")
    b := filepath.Join(dir, ".env.b")
    if err := os.WriteFile(a, []byte("K=first\n"), 0o600); err != nil { t.Fatalf("write a: %v", err) }
    if err := os.WriteFile(b, []byte("K=second\n"), 0o600); err != nil { t.Fatalf("write b: %v", err) }

    if err := LoadEnvFiles(a, b); err != nil {
        t.Fatalf("LoadEnvFiles error: %v", err)
    }
    if got := os.Getenv("K"); got != "second" {
        t.Fatalf("override order failed: got %q, want second", got)
    }
}

// Verify ApplyEnvToConfig reads key settings from environment, including
// SEARXNG_URL fallback and SOURCE_CAPS parsing.
func TestApplyEnvToConfig_FromEnv(t *testing.T) {
    t.Setenv("SEARX_URL", "")
    t.Setenv("SEARXNG_URL", "http://searxng.example")
    t.Setenv("CACHE_DIR", "/tmp/goresearch-cache")
    t.Setenv("LANGUAGE", "fi")
    t.Setenv("SOURCE_CAPS", "9,2")

    var cfg Config
    ApplyEnvToConfig(&cfg)
    if cfg.SearxURL != "http://searxng.example" {
        t.Fatalf("SearxURL=%q, want fallback from SEARXNG_URL", cfg.SearxURL)
    }
    if cfg.CacheDir != "/tmp/goresearch-cache" {
        t.Fatalf("CacheDir=%q, want /tmp/goresearch-cache", cfg.CacheDir)
    }
    if cfg.LanguageHint != "fi" {
        t.Fatalf("LanguageHint=%q, want fi", cfg.LanguageHint)
    }
    if cfg.MaxSources != 9 || cfg.PerDomainCap != 2 {
        t.Fatalf("SOURCE_CAPS parsed to MaxSources=%d PerDomainCap=%d, want 9,2", cfg.MaxSources, cfg.PerDomainCap)
    }
}

// Verify that VERIFY/NO_VERIFY environment variables control DisableVerify.
func TestApplyEnvOverrides_VerificationToggles(t *testing.T) {
    t.Setenv("VERIFY", "true")
    t.Setenv("NO_VERIFY", "")
    cfg := Config{DisableVerify: true}
    ApplyEnvOverrides(&cfg)
    if cfg.DisableVerify {
        t.Fatalf("VERIFY=true should enable verification (DisableVerify=false)")
    }
    t.Setenv("VERIFY", "")
    t.Setenv("NO_VERIFY", "true")
    cfg = Config{}
    ApplyEnvOverrides(&cfg)
    if !cfg.DisableVerify {
        t.Fatalf("NO_VERIFY=true should disable verification")
    }
}
