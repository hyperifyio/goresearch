package robots

import (
    "context"
    "net/http"
    "net/http/httptest"
    "sync/atomic"
    "testing"
    "time"

    "github.com/hyperifyio/goresearch/internal/cache"
)

func TestManager_FetchOncePerRun_WithETagRevalidation(t *testing.T) {
    t.Parallel()
    var hits int32
    const etag = "W/\"v1\""
    body := "User-agent: *\nDisallow: /private\n"
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/robots.txt" {
            http.NotFound(w, r)
            return
        }
        atomic.AddInt32(&hits, 1)
        if r.Header.Get("If-None-Match") == etag {
            w.Header().Set("ETag", etag)
            w.WriteHeader(http.StatusNotModified)
            return
        }
        w.Header().Set("Content-Type", "text/plain")
        w.Header().Set("ETag", etag)
        _, _ = w.Write([]byte(body))
    }))
    t.Cleanup(srv.Close)

    ctx := context.Background()
    dir := t.TempDir()
    m := &Manager{
        HTTPClient:       srv.Client(),
        Cache:            &cache.HTTPCache{Dir: dir},
        UserAgent:        "goresearch-test/1.0",
        EntryExpiry:      time.Hour,
        AllowPrivateHosts: true,
    }

    u := srv.URL + "/robots.txt"
    rules1, src1, err := m.Get(ctx, u)
    if err != nil {
        t.Fatalf("first get: %v", err)
    }
    if src1 != SourceNetwork {
        t.Fatalf("expected SourceNetwork on first get, got %v", src1)
    }
    if len(rules1.Groups) == 0 || len(rules1.Groups[0].Agents) == 0 {
        t.Fatalf("parsed rules missing agents")
    }
    if got := rules1.Groups[0].Disallow[0]; got != "/private" {
        t.Fatalf("unexpected disallow: %q", got)
    }

    // Second fetch within expiry should not hit server
    _, src2, err := m.Get(ctx, u)
    if err != nil {
        t.Fatalf("second get: %v", err)
    }
    if src2 != SourceMemory {
        t.Fatalf("expected SourceMemory on second get, got %v", src2)
    }
    if atomic.LoadInt32(&hits) != 1 {
        t.Fatalf("expected 1 server hit, got %d", hits)
    }

    // Expire and force conditional revalidation; server will return 304
    m.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
    rules3, src3, err := m.Get(ctx, u)
    if err != nil {
        t.Fatalf("third get: %v", err)
    }
    if src3 != SourceCache304 {
        t.Fatalf("expected SourceCache304 on third get, got %v", src3)
    }
    if atomic.LoadInt32(&hits) != 2 {
        t.Fatalf("expected 2 server hits (200 then 304), got %d", hits)
    }
    if rules3.Groups[0].Disallow[0] != "/private" {
        t.Fatalf("rules changed unexpectedly after revalidation")
    }
}

func TestManager_RevalidateWithLastModified(t *testing.T) {
    t.Parallel()
    var hits int32
    lastMod := time.Now().UTC().Format(http.TimeFormat)
    body := "User-agent: *\nAllow: /public\n"
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/robots.txt" {
            http.NotFound(w, r)
            return
        }
        atomic.AddInt32(&hits, 1)
        if ims := r.Header.Get("If-Modified-Since"); ims != "" {
            w.Header().Set("Last-Modified", lastMod)
            w.WriteHeader(http.StatusNotModified)
            return
        }
        w.Header().Set("Content-Type", "text/plain")
        w.Header().Set("Last-Modified", lastMod)
        _, _ = w.Write([]byte(body))
    }))
    t.Cleanup(srv.Close)

    ctx := context.Background()
    dir := t.TempDir()
    m := &Manager{
        HTTPClient:       srv.Client(),
        Cache:            &cache.HTTPCache{Dir: dir},
        UserAgent:        "goresearch-test/1.0",
        EntryExpiry:      time.Minute,
        AllowPrivateHosts: true,
    }
    u := srv.URL + "/robots.txt"
    if _, _, err := m.Get(ctx, u); err != nil {
        t.Fatalf("first get: %v", err)
    }
    if atomic.LoadInt32(&hits) != 1 {
        t.Fatalf("expected 1 server hit, got %d", hits)
    }
    // Expire and fetch again; expect conditional 304 path and no change
    m.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
    rules, src, err := m.Get(ctx, u)
    if err != nil {
        t.Fatalf("second get: %v", err)
    }
    if src != SourceCache304 {
        t.Fatalf("expected SourceCache304, got %v", src)
    }
    if len(rules.Groups) == 0 || len(rules.Groups[0].Allow) == 0 || rules.Groups[0].Allow[0] != "/public" {
        t.Fatalf("unexpected parsed rules after 304 revalidation")
    }
}
