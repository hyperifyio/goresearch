package devops

import (
	"testing"
	"strings"
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

// TestDockerCompose_TLSProfile verifies the TLS profile is correctly configured
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestDockerCompose_TLSProfile(t *testing.T) {
	// Read and parse docker-compose.yml
	composeData, err := ioutil.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}

	var compose map[string]interface{}
	if err := yaml.Unmarshal(composeData, &compose); err != nil {
		t.Fatalf("failed to parse docker-compose.yml: %v", err)
	}

	services, ok := compose["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("no services section found in docker-compose.yml")
	}

    // In the new layout, TLS proxy lives in the optional compose file. Skip here if absent.
    caddyService, exists := services["caddy-tls"].(map[string]interface{})
    if !exists {
        t.Skip("caddy-tls is defined in docker-compose.optional.yml; skipping base compose check")
    }

	// Verify it has TLS profile
	profiles, ok := caddyService["profiles"].([]interface{})
	if !ok {
		t.Fatalf("caddy-tls service missing profiles")
	}
	hasTLSProfile := false
	for _, profile := range profiles {
		if profile.(string) == "tls" {
			hasTLSProfile = true
			break
		}
	}
	if !hasTLSProfile {
		t.Errorf("caddy-tls service missing 'tls' profile")
	}

	// Verify it has correct image with digest
	image, ok := caddyService["image"].(string)
	if !ok {
		t.Fatalf("caddy-tls service missing image")
	}
	if !strings.Contains(image, "caddy:") || !strings.Contains(image, "@sha256:") {
		t.Errorf("caddy-tls service image should be pinned by digest: %s", image)
	}

	// Verify it has Caddyfile volume mount
	volumes, ok := caddyService["volumes"].([]interface{})
	if !ok {
		t.Fatalf("caddy-tls service missing volumes")
	}
	hasCaddyfile := false
	for _, vol := range volumes {
		if strings.Contains(vol.(string), "Caddyfile") {
			hasCaddyfile = true
			break
		}
	}
	if !hasCaddyfile {
		t.Errorf("caddy-tls service missing Caddyfile volume mount")
	}

    // research-tool-tls is not used anymore; CLI runs on host. Ensure it's absent.
    if _, exists := services["research-tool-tls"]; exists {
        t.Fatalf("research-tool-tls should not be defined in base compose")
    }

	// Test 3: Verify dependencies for TLS profile services include required services
    caddyDependsOn, ok := caddyService["depends_on"].(map[string]interface{})
	if !ok {
		t.Fatalf("caddy-tls service missing depends_on")
	}
	
	if _, exists := caddyDependsOn["llm-openai"]; !exists {
		t.Errorf("caddy-tls service should depend on llm-openai")
	}
	if _, exists := caddyDependsOn["searxng"]; !exists {
		t.Errorf("caddy-tls service should depend on searxng")
	}
}

// TestCaddyfile_Configuration verifies the Caddyfile has correct TLS configuration
func TestCaddyfile_Configuration(t *testing.T) {
	// Read Caddyfile
	caddyData, err := ioutil.ReadFile("../../devops/Caddyfile")
	if err != nil {
		t.Fatalf("failed to read Caddyfile: %v", err)
	}

	content := string(caddyData)
	
	// Verify it has :8443 and :8444 server blocks
	if !strings.Contains(content, ":8443") {
		t.Errorf("Caddyfile missing :8443 server block for LLM service")
	}
	if !strings.Contains(content, ":8444") {
		t.Errorf("Caddyfile missing :8444 server block for SearxNG service")
	}
	
	// Verify it has TLS internal configuration
	if !strings.Contains(content, "tls internal") {
		t.Errorf("Caddyfile missing 'tls internal' configuration for self-signed certs")
	}
	
	// Verify it has reverse proxy directives to the correct services
	if !strings.Contains(content, "reverse_proxy llm-openai:8080") {
		t.Errorf("Caddyfile missing reverse proxy to llm-openai:8080")
	}
	if !strings.Contains(content, "reverse_proxy searxng:8080") {
		t.Errorf("Caddyfile missing reverse proxy to searxng:8080")
	}
	
	// Verify it has security headers
	if !strings.Contains(content, "Strict-Transport-Security") {
		t.Errorf("Caddyfile missing HSTS security headers")
	}
}