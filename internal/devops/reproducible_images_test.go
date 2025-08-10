package devops

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestCompose_AllServiceImagesPinnedByDigest enforces reproducibility by ensuring
// every service image in docker-compose.yml is pinned by digest (image@sha256:...).
//
// Traceability: FEATURE_CHECKLIST item 215
// https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestCompose_AllServiceImagesPinnedByDigest(t *testing.T) {
    root := findRepoRoot(t)
    composePath := filepath.Join(root, "docker-compose.yml")
    b, err := os.ReadFile(composePath)
    if err != nil { t.Fatalf("read compose: %v", err) }

    // Very small YAML-less heuristic to keep this test simple and robust:
    // scan for lines starting with two spaces then "image:" and assert they
    // contain an @sha256: digest suffix.
    lines := strings.Split(string(b), "\n")
    var missing []string
    var currentService string
    for _, line := range lines {
        // Track the last seen service key for friendlier messages
        if strings.HasPrefix(line, "  ") && strings.HasSuffix(strings.TrimSpace(line), ":") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
            trimmed := strings.TrimSpace(line)
            if !strings.Contains(trimmed, " ") && !strings.Contains(trimmed, "\t") && !strings.EqualFold(trimmed, "services:") {
                currentService = strings.TrimSuffix(trimmed, ":")
            }
        }
        // image line like "    image: xyz"
        if strings.HasPrefix(line, "    image:") {
            if !strings.Contains(line, "@sha256:") {
                name := currentService
                if name == "" { name = "<unknown>" }
                missing = append(missing, name+" -> "+strings.TrimSpace(line))
            }
        }
    }
    if len(missing) > 0 {
        t.Fatalf("all service images must be pinned by digest; missing: %v", missing)
    }
}

// TestDockerfile_OCITraceabilityLabels ensures the Dockerfile includes standard
// OCI labels for revision (vcs-ref) and created (build-date), wired via build args.
//
// Traceability: FEATURE_CHECKLIST item 215
func TestDockerfile_OCITraceabilityLabels(t *testing.T) {
    root := findRepoRoot(t)
    dockerfilePath := filepath.Join(root, "Dockerfile")
    b, err := os.ReadFile(dockerfilePath)
    if err != nil { t.Fatalf("read Dockerfile: %v", err) }
    s := string(b)
    if !strings.Contains(s, "org.opencontainers.image.revision") {
        t.Fatalf("Dockerfile should label org.opencontainers.image.revision (vcs-ref)")
    }
    if !strings.Contains(s, "org.opencontainers.image.created") {
        t.Fatalf("Dockerfile should label org.opencontainers.image.created (build-date)")
    }
    if !strings.Contains(s, "ARG COMMIT") || !strings.Contains(s, "ARG DATE") {
        t.Fatalf("Dockerfile should declare ARG COMMIT and ARG DATE for labels")
    }
}

// TestMake_ImageBuildTarget_UsesBuildxWithSBOM verifies we provide a convenient
// make target that builds the goresearch image with BuildKit attestations/SBOM
// and passes version/commit/date as build args for traceability.
//
// Traceability: FEATURE_CHECKLIST item 215
func TestMake_ImageBuildTarget_UsesBuildxWithSBOM(t *testing.T) {
    root := findRepoRoot(t)
    makefilePath := filepath.Join(root, "Makefile")
    b, err := os.ReadFile(makefilePath)
    if err != nil { t.Fatalf("Makefile missing: %v", err) }
    mk := string(b)
    if !strings.Contains(mk, "\nimage:") {
        t.Fatalf("Makefile should define an 'image' target for building the container image")
    }
    if !strings.Contains(mk, "docker buildx build") {
        t.Fatalf("image target should use docker buildx build")
    }
    if !strings.Contains(mk, "--sbom") || !strings.Contains(mk, "--provenance") {
        t.Fatalf("image target should enable BuildKit SBOM and provenance attestations")
    }
    if !strings.Contains(mk, "--build-arg VERSION=") || !strings.Contains(mk, "--build-arg COMMIT=") || !strings.Contains(mk, "--build-arg DATE=") {
        t.Fatalf("image target should pass VERSION, COMMIT, and DATE build args")
    }
}
