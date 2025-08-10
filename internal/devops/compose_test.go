package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// TestCompose_LLMServiceConfiguration verifies that the docker-compose file defines
// an OpenAI-compatible LLM server with:
// - image pinned by digest
// - readiness healthcheck on /v1/models
// - a mounted models volume
// - research-tool depends on llm-openai becoming healthy
// This is a static config test and does not require Docker.
func TestCompose_LLMServiceConfiguration(t *testing.T) {
    // Locate compose at repo root
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil {
        t.Fatalf("read compose: %v", err)
    }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil {
        t.Fatalf("yaml unmarshal: %v", err)
    }

    // services map
    services, ok := doc["services"].(map[string]any)
    if !ok {
        t.Fatalf("services missing or wrong type")
    }
    llm, ok := services["llm-openai"].(map[string]any)
    if !ok {
        t.Fatalf("llm-openai service missing")
    }

    // image pinned by digest
    image, _ := llm["image"].(string)
    if image == "" || !strings.Contains(image, "@sha256:") {
        t.Fatalf("llm-openai image must be pinned by digest, got %q", image)
    }

    // healthcheck exists and targets /v1/models
    hc, ok := llm["healthcheck"].(map[string]any)
    if !ok {
        t.Fatalf("llm-openai healthcheck missing")
    }
    testCmd, ok := hc["test"].([]any)
    if !ok || len(testCmd) < 4 {
        t.Fatalf("healthcheck.test malformed: %#v", hc["test"])
    }
    okURL := false
    for _, v := range testCmd {
        if s, ok := v.(string); ok && strings.Contains(s, "/v1/models") {
            okURL = true
            break
        }
    }
    if !okURL {
        t.Fatalf("healthcheck must probe /v1/models; test=%v", testCmd)
    }

    // models volume mount present
    vols, _ := llm["volumes"].([]any)
    foundModels := false
    for _, v := range vols {
        if s, ok := v.(string); ok && strings.Contains(s, "/models") {
            foundModels = true
            break
        }
    }
    if !foundModels {
        t.Fatalf("llm-openai should mount a models volume to /models; volumes=%v", vols)
    }

    // research-tool depends_on llm-openai healthy
    tool, ok := services["research-tool"].(map[string]any)
    if !ok {
        t.Fatalf("research-tool service missing")
    }
    dep, ok := tool["depends_on"].(map[string]any)
    if !ok {
        t.Fatalf("research-tool.depends_on missing or wrong type")
    }
    llmDep, ok := dep["llm-openai"].(map[string]any)
    if !ok {
        t.Fatalf("research-tool.depends_on.llm-openai missing")
    }
    cond, _ := llmDep["condition"].(string)
    if cond != "service_healthy" {
        t.Fatalf("research-tool should depend on llm-openai service_healthy, got %q", cond)
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
