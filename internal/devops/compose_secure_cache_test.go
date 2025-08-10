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
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }

    // volumes.secure_cache exists
    vols, _ := doc["volumes"].(map[string]any)
    if _, ok := vols["secure_cache"]; !ok {
        t.Fatalf("volumes.secure_cache missing")
    }

    services, _ := doc["services"].(map[string]any)

    // research-tool-secure service
    rts, ok := services["research-tool-secure"].(map[string]any)
    if !ok { t.Fatalf("research-tool-secure service missing") }
    // profiles include secure-cache
    if profs, _ := rts["profiles"].([]any); !containsString(profs, "secure-cache") {
        t.Fatalf("research-tool-secure should include 'secure-cache' profile; got %v", profs)
    }
    // environment includes CACHE_STRICT_PERMS=1
    env, _ := rts["environment"].([]any)
    if !hasEnv(env, "CACHE_STRICT_PERMS") {
        t.Fatalf("research-tool-secure must set CACHE_STRICT_PERMS=1; env=%v", env)
    }
    // volumes include secure_cache mount
    rtsVols, _ := rts["volumes"].([]any)
    if !anyStringContains(rtsVols, "secure_cache:/app/.goresearch-cache") {
        t.Fatalf("research-tool-secure should mount secure_cache to /app/.goresearch-cache; vols=%v", rtsVols)
    }

    // perms-init-secure service
    pis, ok := services["perms-init-secure"].(map[string]any)
    if !ok { t.Fatalf("perms-init-secure service missing") }
    if profs, _ := pis["profiles"].([]any); !containsString(profs, "secure-cache") {
        t.Fatalf("perms-init-secure should include 'secure-cache' profile; got %v", profs)
    }
    pisVols, _ := pis["volumes"].([]any)
    if !anyStringContains(pisVols, "secure_cache:/app/.goresearch-cache") {
        t.Fatalf("perms-init-secure should mount secure_cache to /app/.goresearch-cache; vols=%v", pisVols)
    }

    // llm-openai, models-init, and searxng should participate in secure-cache
    for _, svc := range []string{"llm-openai", "models-init", "searxng"} {
        s, ok := services[svc].(map[string]any)
        if !ok { t.Fatalf("%s missing", svc) }
        if profs, _ := s["profiles"].([]any); !containsString(profs, "secure-cache") {
            t.Fatalf("%s should include 'secure-cache' profile; got %v", svc, profs)
        }
    }
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
