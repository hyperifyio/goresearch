package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// TestCompose_ModelsVolumeAndInit validates that a dedicated models volume exists
// and that an optional one-shot init service prepares the volume with checksum
// verification, gating dependent services until it succeeds.
//
// Traceability: Implements FEATURE_CHECKLIST.md item "Model weights volume & bootstrap".
// https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestCompose_ModelsVolumeAndInit(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }

    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil {
        t.Fatalf("yaml unmarshal: %v", err)
    }

    // volumes must include a named "models" volume
    vols, ok := doc["volumes"].(map[string]any)
    if !ok {
        t.Fatalf("volumes missing or wrong type")
    }
    if _, ok := vols["models"]; !ok {
        t.Fatalf("expected named volume 'models' to be defined")
    }

    // services
    services, ok := doc["services"].(map[string]any)
    if !ok { t.Fatalf("services missing or wrong type") }

    // llm-openai should mount models:/models
    llm, ok := services["llm-openai"].(map[string]any)
    if !ok { t.Fatalf("llm-openai service missing") }
    llmVols, _ := llm["volumes"].([]any)
    foundModelsMount := false
    for _, v := range llmVols {
        if s, ok := v.(string); ok && strings.HasPrefix(s, "models:") && strings.Contains(s, "/models") {
            foundModelsMount = true
            break
        }
    }
    if !foundModelsMount {
        t.Fatalf("llm-openai should mount models:/models; volumes=%v", llmVols)
    }

    // optional init service should exist and mount models, run checksum verification,
    // and be used as a completion gate for llm-openai and research-tool
    initSvc, ok := services["models-init"].(map[string]any)
    if !ok {
        t.Fatalf("models-init service missing (expected one-shot bootstrap)")
    }
    initVols, _ := initSvc["volumes"].([]any)
    foundInitModelsMount := false
    for _, v := range initVols {
        if s, ok := v.(string); ok && strings.HasPrefix(s, "models:") && strings.Contains(s, "/models") {
            foundInitModelsMount = true
            break
        }
    }
    if !foundInitModelsMount {
        t.Fatalf("models-init should mount models:/models; volumes=%v", initVols)
    }
    // command should reference checksum verification
    switch cmd := initSvc["command"].(type) {
    case []any:
        hasChecksum := false
        for _, v := range cmd {
            if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), "sha256sum") {
                hasChecksum = true
                break
            }
        }
        if !hasChecksum { t.Fatalf("models-init command should include sha256sum verification; command=%v", cmd) }
    case string:
        if !strings.Contains(strings.ToLower(cmd), "sha256sum") {
            t.Fatalf("models-init command should include sha256sum verification; command=%v", cmd)
        }
    default:
        t.Fatalf("models-init command unexpected type: %T", cmd)
    }

    // init service should not restart
    if rs, _ := initSvc["restart"].(string); rs != "no" {
        t.Fatalf("models-init should set restart: 'no', got %q", rs)
    }

    // llm-openai must depend on models-init completion, not just existence
    depLLM, _ := llm["depends_on"].(map[string]any)
    if depLLM == nil { t.Fatalf("llm-openai.depends_on missing") }
    initDep, _ := depLLM["models-init"].(map[string]any)
    if initDep == nil { t.Fatalf("llm-openai.depends_on.models-init missing") }
    if cond, _ := initDep["condition"].(string); cond != "service_completed_successfully" {
        t.Fatalf("llm-openai should depend on models-init service_completed_successfully, got %q", cond)
    }

    // research-tool should also wait for models-init
    tool, ok := services["research-tool"].(map[string]any)
    if !ok { t.Fatalf("research-tool service missing") }
    depTool, _ := tool["depends_on"].(map[string]any)
    if depTool == nil { t.Fatalf("research-tool.depends_on missing") }
    initDepTool, _ := depTool["models-init"].(map[string]any)
    if initDepTool == nil { t.Fatalf("research-tool.depends_on.models-init missing") }
    if cond, _ := initDepTool["condition"].(string); cond != "service_completed_successfully" {
        t.Fatalf("research-tool should depend on models-init service_completed_successfully, got %q", cond)
    }
}

// TestModelsBootstrapScript asserts the bootstrap script exists and performs
// checksum verification to fail clearly on mismatches.
func TestModelsBootstrapScript(t *testing.T) {
    root := findRepoRoot(t)
    scriptPath := filepath.Join(root, "scripts", "bootstrap-models.sh")
    b, err := os.ReadFile(scriptPath)
    if err != nil {
        t.Fatalf("bootstrap script missing: %v", err)
    }
    content := string(b)
    if !strings.Contains(content, "set -euo pipefail") {
        t.Fatalf("bootstrap script should enable strict mode")
    }
    if !strings.Contains(strings.ToLower(content), "sha256sum -c") {
        t.Fatalf("bootstrap script should verify checksums with 'sha256sum -c'")
    }
}
