package app

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// newHighThroughputHTTPClient returns an HTTP client tuned for high parallelism
// without client-side throttling. Timeouts are kept reasonable to avoid hangs.
// If sslVerify is false, SSL certificate verification is disabled for self-signed certs.
func newHighThroughputHTTPClient(sslVerify bool) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          0,    // no global limit
		MaxIdleConnsPerHost:   1024, // large per-host pool
		MaxConnsPerHost:       0,    // unlimited
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Configure TLS settings based on SSL verification preference
	if !sslVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}
