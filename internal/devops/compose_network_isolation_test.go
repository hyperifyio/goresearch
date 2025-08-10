package devops

import (
    "os"
    "path/filepath"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// TestCompose_NetworkIsolation verifies that all services are attached only to
// the private internal network and that no service publishes host ports by default.
//
// Traceability: Implements FEATURE_CHECKLIST.md item 225:
// "Network isolation — Use a private Compose network; do not publish ports by default.
//  The tool reaches only llm-openai and searxng by service name. Document an override
//  file to expose ports when needed."
func TestCompose_NetworkIsolation(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }

    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }

    // networks.goresearch_net must be internal
    nets, _ := doc["networks"].(map[string]any)
    if nets == nil { t.Fatalf("networks missing") }
    goresearchNet, _ := nets["goresearch_net"].(map[string]any)
    if goresearchNet == nil { t.Fatalf("goresearch_net missing") }
    if internal, _ := goresearchNet["internal"].(bool); !internal {
        t.Fatalf("goresearch_net should be internal: true")
    }

    services, _ := doc["services"].(map[string]any)
    if services == nil { t.Fatalf("services missing") }

    for name, raw := range services {
        svc, _ := raw.(map[string]any)
        if svc == nil { t.Fatalf("service %s not a map", name) }

        // Each service should join only goresearch_net
        netsList, _ := svc["networks"].([]any)
        if len(netsList) == 0 {
            t.Fatalf("service %s must specify networks and include goresearch_net", name)
        }
        if len(netsList) != 1 {
            t.Fatalf("service %s should attach only to goresearch_net, got %v", name, netsList)
        }
        if s, ok := netsList[0].(string); !ok || s != "goresearch_net" {
            t.Fatalf("service %s must use goresearch_net, got %v", name, netsList)
        }

        // No ports should be published by default
        if _, has := svc["ports"]; has {
            t.Fatalf("service %s should not publish ports in base compose", name)
        }
    }
}

// TestCompose_OverrideExampleExists verifies we provide a documented override example
// to expose ports when needed without changing the base compose file.
func TestCompose_OverrideExampleExists(t *testing.T) {
    root := findRepoRoot(t)
    overridePath := filepath.Join(root, "docker-compose.override.yml.example")
    b, err := os.ReadFile(overridePath)
    if err != nil {
        t.Fatalf("override example missing: %v", err)
    }

    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }
    services, _ := doc["services"].(map[string]any)
    if services == nil { t.Fatalf("services missing in override example") }

    // llm-openai has ports
    if llm, _ := services["llm-openai"].(map[string]any); llm == nil {
        t.Fatalf("llm-openai missing in override example")
    } else if _, ok := llm["ports"]; !ok {
        t.Fatalf("llm-openai should publish ports in override example")
    }

    // searxng has ports
    if searx, _ := services["searxng"].(map[string]any); searx == nil {
        t.Fatalf("searxng missing in override example")
    } else if _, ok := searx["ports"]; !ok {
        t.Fatalf("searxng should publish ports in override example")
    }
}
