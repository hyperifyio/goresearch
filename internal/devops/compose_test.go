package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// No longer applicable: optional LLM service has been removed from compose.
// Keep a placeholder test to assert that no LLM service exists in optional compose.
func TestCompose_NoLLMService(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.optional.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }
    services, ok := doc["services"].(map[string]any)
    if !ok { t.Fatalf("services missing or wrong type") }
    if _, exists := services["llm-openai"]; exists {
        t.Fatalf("llm-openai service must not be defined in optional compose")
    }
}

func findRepoRoot(t *testing.T) string {
    t.Helper()
    dir, err := os.Getwd()
    if err != nil { t.Fatalf("getwd: %v", err) }
    // Walk up until we find go.mod
    for i := 0; i < 5; i++ {
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir { break }
        dir = parent
    }
    t.Fatalf("could not locate repo root with go.mod")
    return ""
}
