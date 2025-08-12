package devops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMake_WaitTargetAndScript verifies the presence of a Makefile wait target
// and a health polling script that checks LLM /v1/models and SearxNG /status.
//
// Traceability: Implements FEATURE_CHECKLIST.md item
// "Health-gated startup — Use depends_on with condition: service_healthy so the tool starts only after llm-openai and searxng are ready. Provide a make wait target that polls health for local troubleshooting."
func TestMake_WaitTargetAndScript(t *testing.T) {
	root := findRepoRoot(t)

	// Check Makefile exists and has a wait target invoking the script
	makefilePath := filepath.Join(root, "Makefile")
	b, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("Makefile missing: %v", err)
	}
	mk := string(b)
	if !strings.Contains(mk, "\nwait:") {
		t.Fatalf("Makefile should define a 'wait' target")
	}
	if !strings.Contains(mk, "scripts/wait-for-health.sh") {
		t.Fatalf("wait target should invoke scripts/wait-for-health.sh")
	}

	// Check script exists and probes correct endpoints with curl -fsS
	scriptPath := filepath.Join(root, "scripts", "wait-for-health.sh")
	b, err = os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("wait-for-health.sh missing: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "set -euo pipefail") {
		t.Fatalf("wait-for-health.sh should enable strict mode")
	}
    if !strings.Contains(content, "/v1/models") && !strings.Contains(content, "/models") {
		t.Fatalf("script should probe LLM /v1/models endpoint")
	}
    // Accept either /status probe or root (/) probe for SearxNG since some builds
    // disable /status endpoint in recent versions.
    if !(strings.Contains(content, "/status") || strings.Contains(content, "SearxNG root")) {
        t.Fatalf("script should probe SearxNG health (root or /status) endpoint")
    }
	if !strings.Contains(content, "curl -fsS") {
		t.Fatalf("script should use curl -fsS for health checks")
	}
}
