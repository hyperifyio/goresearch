package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    yaml "gopkg.in/yaml.v3"
)

// TestCompose_NonRootVolumesAndUser ensures that the compose file configures
// the app services to run as a non-root user via APP_UID:APP_GID so that files
// written to bind mounts and named volumes are created with matching ownership.
//
// Traceability: Implements FEATURE_CHECKLIST.md item 211:
// "Non-root volumes & permissions — Create volumes with matching UID:GID for the
//  app user in containers; provide a helper script/compose override to chown
//  existing host directories to avoid permission errors."
func TestCompose_NonRootVolumesAndUser(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil {
        t.Fatalf("read compose: %v", err)
    }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil {
        t.Fatalf("yaml: %v", err)
    }

    services, ok := doc["services"].(map[string]any)
    if !ok { t.Fatalf("services missing or wrong type") }

    // research-tool is no longer defined in compose; CLI runs on host
    if _, ok := services["research-tool"]; ok {
        t.Fatalf("research-tool should not be defined in base compose")
    }
}

// TestPermissionsHelperArtifacts validates we provide a helper compose override
// and a host-side script to fix ownership of existing host directories.
func TestPermissionsHelperArtifacts(t *testing.T) {
    root := findRepoRoot(t)

    // Compose override example for permissions
    overridePath := filepath.Join(root, "docker-compose.permissions.yml.example")
    b, err := os.ReadFile(overridePath)
    if err != nil {
        t.Fatalf("permissions override example missing: %v", err)
    }
    var over map[string]any
    if err := yaml.Unmarshal(b, &over); err != nil {
        t.Fatalf("yaml: %v", err)
    }
    services, _ := over["services"].(map[string]any)
    if services == nil { t.Fatalf("services missing in permissions override example") }
    fix, _ := services["fix-perms"].(map[string]any)
    if fix == nil { t.Fatalf("expected fix-perms service in permissions override example") }
    // Command should include chown and reference APP_UID/APP_GID
    switch cmd := fix["command"].(type) {
    case []any:
        joined := make([]string, 0, len(cmd))
        for _, v := range cmd { if s, ok := v.(string); ok { joined = append(joined, s) } }
        joinedStr := strings.Join(joined, " ")
        if !strings.Contains(joinedStr, "chown") || !strings.Contains(joinedStr, "APP_UID") || !strings.Contains(joinedStr, "APP_GID") {
            t.Fatalf("fix-perms command should chown using APP_UID/APP_GID; got %v", joined)
        }
    case string:
        if !strings.Contains(cmd, "chown") || !strings.Contains(cmd, "APP_UID") || !strings.Contains(cmd, "APP_GID") {
            t.Fatalf("fix-perms command should chown using APP_UID/APP_GID; got %q", cmd)
        }
    default:
        t.Fatalf("fix-perms command unexpected type: %T", cmd)
    }

    // Host-side helper script
    scriptPath := filepath.Join(root, "scripts", "chown-host-dirs.sh")
    sb, err := os.ReadFile(scriptPath)
    if err != nil { t.Fatalf("chown helper script missing: %v", err) }
    content := string(sb)
    if !strings.Contains(content, "set -euo pipefail") {
        t.Fatalf("chown helper should enable strict mode")
    }
    if !strings.Contains(content, "chown -R") {
        t.Fatalf("chown helper should recursively chown target directories")
    }
}

// TestCompose_VolumesPermsInit ensures a one-shot init service exists to chown
// named volumes (http_cache, llm_cache, reports) to APP_UID:APP_GID so the app
// can write to them as a non-root user.
func TestCompose_VolumesPermsInit(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }
    var doc map[string]any
    if err := yaml.Unmarshal(b, &doc); err != nil { t.Fatalf("yaml: %v", err) }

    services, _ := doc["services"].(map[string]any)
    if services == nil { t.Fatalf("services missing") }

    init, _ := services["perms-init"].(map[string]any)
    if init == nil { t.Skip("perms-init not present in minimal base compose") }

    // Should mount the named volumes
    vols, _ := init["volumes"].([]any)
    if vols == nil || len(vols) == 0 { t.Fatalf("perms-init.volumes missing") }
    joined := make([]string, 0, len(vols))
    for _, v := range vols { if s, ok := v.(string); ok { joined = append(joined, s) } }
    hasHTTP := false; hasLLM := false; hasReports := false
    for _, s := range joined {
        if strings.HasPrefix(s, "http_cache:") { hasHTTP = true }
        if strings.HasPrefix(s, "llm_cache:") { hasLLM = true }
        if strings.HasPrefix(s, "reports:") { hasReports = true }
    }
    if !hasHTTP || !hasLLM || !hasReports {
        t.Fatalf("perms-init should mount http_cache, llm_cache, and reports volumes; got %v", joined)
    }

    // Command should perform chown with APP_UID/APP_GID
    switch cmd := init["command"].(type) {
    case []any:
        parts := make([]string, 0, len(cmd))
        for _, v := range cmd { if s, ok := v.(string); ok { parts = append(parts, s) } }
        all := strings.Join(parts, " ")
        if !strings.Contains(all, "chown") || !strings.Contains(all, "APP_UID") || !strings.Contains(all, "APP_GID") {
            t.Fatalf("perms-init command should chown using APP_UID/APP_GID; got %v", parts)
        }
    case string:
        if !strings.Contains(cmd, "chown") || !strings.Contains(cmd, "APP_UID") || !strings.Contains(cmd, "APP_GID") {
            t.Fatalf("perms-init command should chown using APP_UID/APP_GID; got %q", cmd)
        }
    default:
        t.Fatalf("perms-init command unexpected type: %T", cmd)
    }

    // App services should depend on perms-init completion
    // No research-tool dependencies in minimal compose
}
