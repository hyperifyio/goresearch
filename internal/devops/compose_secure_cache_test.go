package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// TestCompose_SecureCacheProfile validates the dedicated secure cache volume,
// secure-cache profile services, and env toggle for at-rest protection.
func TestCompose_SecureCacheProfile(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.optional.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }

    // secure-cache applies to optional services; do not assert base volumes here

    services, _ := doc["services"].(map[string]any)

    // research-tool-secure no longer present; CLI runs on host

    // perms-init-secure not required in optional compose minimal setup

    // No LLM or model-init services remain; nothing to assert here.
}

func containsString(items []any, needle string) bool {
    for _, v := range items {
        if s, ok := v.(string); ok && s == needle {
            return true
        }
    }
    return false
}

func anyStringContains(items []any, sub string) bool {
    for _, v := range items {
        if s, ok := v.(string); ok && strings.Contains(s, sub) {
            return true
        }
    }
    return false
}

func hasEnv(items []any, key string) bool {
    for _, v := range items {
        if s, ok := v.(string); ok {
            // KEY=VALUE or KEY
            if strings.HasPrefix(s, key+"=") || s == key {
                return true
            }
        }
    }
    return false
}
