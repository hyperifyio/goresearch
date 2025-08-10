package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// TestCompose_SearxNGServiceConfiguration ensures the SearxNG service is configured
// appropriately for internal-only use with pinned image, healthcheck, and mounted settings.
//
// Traceability: Implements FEATURE_CHECKLIST.md item "SearxNG container — Add a searxng service..."
// https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestCompose_SearxNGServiceConfiguration(t *testing.T) {
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

    // networks.goresearch_net must be internal
    nets, ok := doc["networks"].(map[string]any)
    if !ok {
        t.Fatalf("networks missing")
    }
    net, ok := nets["goresearch_net"].(map[string]any)
    if !ok {
        t.Fatalf("goresearch_net missing")
    }
    internal, _ := net["internal"].(bool)
    if !internal {
        t.Fatalf("goresearch_net should be internal: true")
    }

    services, ok := doc["services"].(map[string]any)
    if !ok {
        t.Fatalf("services missing or wrong type")
    }
    searx, ok := services["searxng"].(map[string]any)
    if !ok {
        t.Fatalf("searxng service missing")
    }

    // image pinned by digest
    image, _ := searx["image"].(string)
    if image == "" || !strings.Contains(image, "@sha256:") {
        t.Fatalf("searxng image must be pinned by digest, got %q", image)
    }

    // healthcheck exists and probes /status
    hc, ok := searx["healthcheck"].(map[string]any)
    if !ok {
        t.Fatalf("searxng healthcheck missing")
    }
    testCmd, ok := hc["test"].([]any)
    if !ok || len(testCmd) < 4 {
        t.Fatalf("searxng healthcheck.test malformed: %#v", hc["test"])
    }
    okURL := false
    for _, v := range testCmd {
        if s, ok := v.(string); ok && strings.Contains(s, "/status") {
            okURL = true
            break
        }
    }
    if !okURL {
        t.Fatalf("searxng healthcheck must probe /status; test=%v", testCmd)
    }

    // volumes include mounted settings.yml
    vols, _ := searx["volumes"].([]any)
    foundSettings := false
    for _, v := range vols {
        if s, ok := v.(string); ok && strings.Contains(s, "/searxng-settings.yml:/etc/searxng/settings.yml") {
            foundSettings = true
            break
        }
    }
    if !foundSettings {
        t.Fatalf("searxng should mount devops/searxng-settings.yml to /etc/searxng/settings.yml; volumes=%v", vols)
    }

    // should not publish ports externally
    if _, hasPorts := searx["ports"]; hasPorts {
        t.Fatalf("searxng should not publish ports to host")
    }

    // research-tool depends_on searxng healthy (online variant)
    tool, ok := services["research-tool"].(map[string]any)
    if !ok {
        t.Fatalf("research-tool service missing")
    }
    dep, ok := tool["depends_on"].(map[string]any)
    if !ok {
        t.Fatalf("research-tool.depends_on missing or wrong type")
    }
    searxDep, ok := dep["searxng"].(map[string]any)
    if !ok {
        t.Fatalf("research-tool.depends_on.searxng missing")
    }
    cond, _ := searxDep["condition"].(string)
    if cond != "service_healthy" {
        t.Fatalf("research-tool should depend on searxng service_healthy, got %q", cond)
    }

    // offline variant should not depend on searxng
    off, ok := services["research-tool-offline"].(map[string]any)
    if !ok { t.Fatalf("research-tool-offline service missing") }
    depOff, _ := off["depends_on"].(map[string]any)
    if depOff == nil { t.Fatalf("research-tool-offline.depends_on missing") }
    if _, has := depOff["searxng"]; has {
        t.Fatalf("offline service must not depend on searxng")
    }
}
