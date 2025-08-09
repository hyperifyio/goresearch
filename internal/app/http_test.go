package app

import (
	"net/http"
	"reflect"
	"testing"
)

func TestNewHighThroughputHTTPClient_Config(t *testing.T) {
	c := newHighThroughputHTTPClient()
	if c.Timeout == 0 {
		t.Fatalf("expected non-zero timeout")
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected http.Transport")
	}
	if tr.MaxIdleConnsPerHost < 100 {
		t.Fatalf("expected large MaxIdleConnsPerHost, got %d", tr.MaxIdleConnsPerHost)
	}
	// Ensure we didn't return the default client's transport
	if reflect.ValueOf(http.DefaultTransport).Pointer() == reflect.ValueOf(tr).Pointer() {
		t.Fatalf("transport should not be default")
	}
}
