package app

import (
	"net/http"
	"testing"
)

// TestNewHighThroughputHTTPClient_SSLVerifyEnabled tests that SSL verification
// is enabled by default (sslVerify=true).
func TestNewHighThroughputHTTPClient_SSLVerifyEnabled(t *testing.T) {
	client := newHighThroughputHTTPClient(true)
	
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	
	// When SSL verification is enabled, TLSClientConfig should be nil or have InsecureSkipVerify=false
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("expected SSL verification to be enabled, but InsecureSkipVerify=true")
	}
}

// TestNewHighThroughputHTTPClient_SSLVerifyDisabled tests that SSL verification
// can be disabled for self-signed certificates (sslVerify=false).
func TestNewHighThroughputHTTPClient_SSLVerifyDisabled(t *testing.T) {
	client := newHighThroughputHTTPClient(false)
	
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	
	// When SSL verification is disabled, TLSClientConfig should have InsecureSkipVerify=true
	if transport.TLSClientConfig == nil {
		t.Errorf("expected TLSClientConfig to be set when SSL verification is disabled")
		return
	}
	
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify=true when SSL verification is disabled, got false")
	}
}

// TestSSLVerifyConfig_Default tests that the default SSL verify configuration is true.
func TestSSLVerifyConfig_Default(t *testing.T) {
	cfg := Config{}
	
	// The default value should be false (zero value for bool)
	// but the CLI flag parsing sets it to true by default via getenv("SSL_VERIFY") != "false"
	if cfg.SSLVerify {
		t.Errorf("expected default SSLVerify to be false (zero value), got true")
	}
}

// TestSSLVerifyConfig_Explicit tests explicit SSL verify configuration values.
func TestSSLVerifyConfig_Explicit(t *testing.T) {
	tests := []struct {
		name     string
		sslVerify bool
		wantSkip bool
	}{
		{"SSL verification enabled", true, false},
		{"SSL verification disabled", false, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{SSLVerify: tt.sslVerify}
			client := newHighThroughputHTTPClient(cfg.SSLVerify)
			
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("expected *http.Transport, got %T", client.Transport)
			}
			
			var actualSkip bool
			if transport.TLSClientConfig != nil {
				actualSkip = transport.TLSClientConfig.InsecureSkipVerify
			}
			
			if actualSkip != tt.wantSkip {
				t.Errorf("SSLVerify=%v: expected InsecureSkipVerify=%v, got %v", 
					tt.sslVerify, tt.wantSkip, actualSkip)
			}
		})
	}
}