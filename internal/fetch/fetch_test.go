package fetch

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    "github.com/hyperifyio/goresearch/internal/cache"
)

func TestRejectsCredentialsInURL(t *testing.T) {
    c := &Client{}
    if _, _, _, _, _, err := c.tryOnce(context.Background(), "http://user:pass@example.com", "", ""); err == nil {
        t.Fatalf("expected credentials rejection")
    }
}

func TestRejectsLocalhostAndPrivateIPs(t *testing.T) {
    c := &Client{}
    cases := []string{
        "http://localhost/",
        "http://127.0.0.1/",
        "http://[::1]/",
        "http://10.0.0.1/",
        "http://192.168.1.2/",
        "http://169.254.1.1/",
    }
    for _, u := range cases {
        if _, _, _, _, _, err := c.tryOnce(context.Background(), u, "", ""); err == nil {
            t.Fatalf("expected rejection for %s", u)
        }
    }
}

func TestGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 2, PerRequestTimeout: 2 * time.Second, AllowPrivateHosts: true}
	body, ct, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct == "" || string(body) == "" {
		t.Fatalf("expected content type and body")
	}
}

func TestGet_RetryOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(502)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 2, PerRequestTimeout: 2 * time.Second, AllowPrivateHosts: true}
	_, _, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
}

func TestGet_Conditional304_UsesCache(t *testing.T) {
	// First return 200 with ETag. Subsequent requests that include If-None-Match should get 304.
	var calls int
	etag := `"abc123"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/html")
		if calls == 1 {
			w.Header().Set("ETag", etag)
			_, _ = w.Write([]byte("first"))
			return
		}
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// Should not happen if cache sends conditional headers
		fmt.Fprintln(w, "unexpected")
	}))
	defer srv.Close()

	tmp := t.TempDir()
    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 1, PerRequestTimeout: 2 * time.Second, Cache: &cache.HTTPCache{Dir: tmp}, AllowPrivateHosts: true}

	// First fetch populates cache
	b1, _, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("first get error: %v", err)
	}
	if string(b1) != "first" {
		t.Fatalf("unexpected body1: %q", string(b1))
	}

	// Second fetch should send conditional headers and get 304, returning cached body
	b2, _, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("second get error: %v", err)
	}
	if string(b2) != "first" {
		t.Fatalf("expected cached body, got %q", string(b2))
	}
}

func TestGet_RejectsNonHTTP(t *testing.T) {
    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 1, PerRequestTimeout: 1 * time.Second, AllowPrivateHosts: true}
	_, _, err := c.Get(context.Background(), "file:///etc/hosts")
	if err == nil {
		t.Fatalf("expected error for non-http scheme")
	}
}

func TestGet_ContentTypeGating(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/html":
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            w.WriteHeader(200)
            _, _ = io.WriteString(w, "<html><body>ok</body></html>")
        case "/pdf":
            w.Header().Set("Content-Type", "application/pdf")
            w.WriteHeader(200)
            _, _ = w.Write([]byte("%PDF-1.4\n1 0 obj\n<</Type/Catalog>>\nendobj\nstream\n(Hello PDF) Tj\nendstream\n"))
        default:
            w.WriteHeader(404)
        }
    }))
	defer srv.Close()

    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 1, PerRequestTimeout: 2 * time.Second, AllowPrivateHosts: true}
    // HTML allowed
    if _, ct, err := c.Get(context.Background(), srv.URL+"/html"); err != nil || !strings.HasPrefix(ct, "text/html") {
        t.Fatalf("expected html allowed, ct=%q err=%v", ct, err)
    }
    // PDF blocked by default
    if _, _, err := c.Get(context.Background(), srv.URL+"/pdf"); err == nil {
        t.Fatalf("expected error for pdf without EnablePDF")
    }
    // Enable and allow PDF
    c.EnablePDF = true
    if _, ct, err := c.Get(context.Background(), srv.URL+"/pdf"); err != nil || !strings.HasPrefix(ct, "application/pdf") {
        t.Fatalf("expected pdf allowed with EnablePDF, ct=%q err=%v", ct, err)
    }
}

func TestGet_RedirectLimit(t *testing.T) {
	// First path redirects once to /next; with RedirectMaxHops=1 this should fail immediately
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/next", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 1, PerRequestTimeout: 2 * time.Second, RedirectMaxHops: 1, AllowPrivateHosts: true}
	_, _, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatalf("expected redirect limit error")
	}
}

func TestGet_MaxConcurrent(t *testing.T) {
	var inFlight int32
	var maxObserved int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		curr := atomic.AddInt32(&inFlight, 1)
		for {
			prev := atomic.LoadInt32(&maxObserved)
			if curr > prev {
				if atomic.CompareAndSwapInt32(&maxObserved, prev, curr) {
					break
				}
				continue
			}
			break
		}
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("ok"))
		atomic.AddInt32(&inFlight, -1)
	}))
	defer srv.Close()

    c := &Client{UserAgent: "goresearch-test", MaxAttempts: 1, PerRequestTimeout: 2 * time.Second, MaxConcurrent: 2, AllowPrivateHosts: true}

	var wg sync.WaitGroup
	start := make(chan struct{})
	num := 6
	wg.Add(num)
	for i := 0; i < num; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, _, _ = c.Get(context.Background(), srv.URL)
		}()
	}
	close(start)
	wg.Wait()

	if maxObserved > 2 {
		t.Fatalf("expected max concurrency <= 2, got %d", maxObserved)
	}
}
