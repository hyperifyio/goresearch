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
