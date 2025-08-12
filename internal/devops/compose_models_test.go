package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// Models volume/bootstrap removed with LocalAI elimination; ensure no model-related
// services remain in optional compose.
func TestCompose_NoModelBootstrapServices(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.optional.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }
    // volumes section may exist but must not contain 'models'
    if vols, ok := doc["volumes"].(map[string]any); ok {
        if _, has := vols["models"]; has {
            t.Fatalf("models volume should not be defined")
        }
    }
    services, _ := doc["services"].(map[string]any)
    if services == nil { t.Fatalf("services missing") }
    if _, ok := services["models-init"]; ok { t.Fatalf("models-init should be removed") }
    if _, ok := services["models-bootstrap"]; ok { t.Fatalf("models-bootstrap should be removed") }
}

// TestModelsBootstrapScript asserts the bootstrap script exists and performs
// checksum verification to fail clearly on mismatches.
// No bootstrap script expected anymore.
