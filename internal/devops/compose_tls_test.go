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

	// Test 1: caddy-tls service exists and has correct configuration
	caddyService, exists := services["caddy-tls"].(map[string]interface{})
	if !exists {
		t.Fatalf("caddy-tls service not found in docker-compose.yml")
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

	// Test 2: research-tool-tls service exists and has correct configuration
	researchService, exists := services["research-tool-tls"].(map[string]interface{})
	if !exists {
		t.Fatalf("research-tool-tls service not found in docker-compose.yml")
	}

	// Verify it has TLS profile
	profiles, ok = researchService["profiles"].([]interface{})
	if !ok {
		t.Fatalf("research-tool-tls service missing profiles")
	}
	hasTLSProfile = false
	for _, profile := range profiles {
		if profile.(string) == "tls" {
			hasTLSProfile = true
			break
		}
	}
	if !hasTLSProfile {
		t.Errorf("research-tool-tls service missing 'tls' profile")
	}

	// Verify it has correct environment variables for HTTPS endpoints
	env, ok := researchService["environment"].([]interface{})
	if !ok {
		t.Fatalf("research-tool-tls service missing environment")
	}
	
	hasHTTPSLLM := false
	hasHTTPSSearx := false
	hasSSLVerifyFalse := false
	
	for _, envVar := range env {
		envStr := envVar.(string)
		if strings.HasPrefix(envStr, "LLM_BASE_URL=https://") {
			hasHTTPSLLM = true
		}
		if strings.HasPrefix(envStr, "SEARX_URL=https://") {
			hasHTTPSSearx = true
		}
		if envStr == "SSL_VERIFY=false" {
			hasSSLVerifyFalse = true
		}
	}
	
	if !hasHTTPSLLM {
		t.Errorf("research-tool-tls service missing HTTPS LLM_BASE_URL")
	}
	if !hasHTTPSSearx {
		t.Errorf("research-tool-tls service missing HTTPS SEARX_URL")
	}
	if !hasSSLVerifyFalse {
		t.Errorf("research-tool-tls service missing SSL_VERIFY=false")
	}

	// Test 3: Verify dependencies for TLS profile services include required services
	dependsOn, ok := researchService["depends_on"].(map[string]interface{})
	if !ok {
		t.Fatalf("research-tool-tls service missing depends_on")
	}
	
	if _, exists := dependsOn["caddy-tls"]; !exists {
		t.Errorf("research-tool-tls service should depend on caddy-tls")
	}
	
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