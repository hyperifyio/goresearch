package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestMake_DXTargets verifies developer experience targets exist in the Makefile
// and reference expected docker compose invocations and cache pruning.
//
// Traceability: Implements FEATURE_CHECKLIST.md item 217
// "Make targets for DX — Add make up, make down, make logs, make rebuild, make test (uses test profile + stub-llm), and make clean (prunes volumes for caches)."
func TestMake_DXTargets(t *testing.T) {
    root := findRepoRoot(t)
    makefilePath := filepath.Join(root, "Makefile")
    b, err := os.ReadFile(makefilePath)
    if err != nil {
        t.Fatalf("Makefile missing: %v", err)
    }
    mk := string(b)

    // Required targets
    for _, target := range []string{"\nup:", "\ndown:", "\nlogs:", "\nrebuild:", "\ntest:", "\nclean:"} {
        if !strings.Contains(mk, target) {
            t.Fatalf("Makefile should define a %q target", strings.TrimSpace(target))
        }
    }

    // up uses compose with dev profile
    if !strings.Contains(mk, "docker compose --profile dev up -d") {
        t.Fatalf("up target should use docker compose with dev profile")
    }

    // rebuild recreates with build
    if !strings.Contains(mk, "--build") || !strings.Contains(mk, "--force-recreate") {
        t.Fatalf("rebuild target should include --build and --force-recreate")
    }

    // logs follows compose logs
    if !strings.Contains(mk, "docker compose logs -f") {
        t.Fatalf("logs target should follow docker compose logs -f")
    }

    // test brings up stub-llm under test profile and runs go test
    if !strings.Contains(mk, "--profile test up -d stub-llm") || !strings.Contains(mk, "go test ./...") {
        t.Fatalf("test target should start stub-llm (test profile) and run go test")
    }

    // clean prunes cache volumes and local cache dir
    if !strings.Contains(mk, "goresearch_http_cache") || !strings.Contains(mk, "goresearch_llm_cache") {
        t.Fatalf("clean target should remove cache volumes goresearch_http_cache and goresearch_llm_cache")
    }
    if !strings.Contains(mk, ".goresearch-cache") {
        t.Fatalf("clean target should also remove local .goresearch-cache directory")
    }
}
