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

// When /robots.txt returns 404, proceed as allowed and cache the negative
// result in memory until expiry so we do not refetch within the window.
func TestMissingRobots404_ProceedAllowed_WithMemCache(t *testing.T) {
    t.Parallel()
    var hits int32
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/robots.txt" {
            http.NotFound(w, r)
            return
        }
        atomic.AddInt32(&hits, 1)
        http.NotFound(w, r)
    }))
    t.Cleanup(srv.Close)

    ctx := context.Background()
    m := &Manager{
        HTTPClient:        srv.Client(),
        UserAgent:         "goresearch-test/1.0",
        EntryExpiry:       time.Minute,
        AllowPrivateHosts: true,
    }
    u := srv.URL + "/robots.txt"
    rules1, src1, err1 := m.Get(ctx, u)
    if err1 != nil {
        t.Fatalf("get 404 robots: %v", err1)
    }
    if src1 != SourceNetwork {
        t.Fatalf("expected SourceNetwork, got %v", src1)
    }
    // An empty ruleset should allow by default
    if allowed := rules1.IsAllowed("goresearch", "/any/path"); !allowed {
        t.Fatalf("expected allow with missing robots 404")
    }
    if atomic.LoadInt32(&hits) != 1 {
        t.Fatalf("expected 1 hit, got %d", hits)
    }
    // Second call should use memory and not refetch
    _, src2, err2 := m.Get(ctx, u)
    if err2 != nil {
        t.Fatalf("second get: %v", err2)
    }
    if src2 != SourceMemory {
        t.Fatalf("expected SourceMemory, got %v", src2)
    }
    if atomic.LoadInt32(&hits) != 1 {
        t.Fatalf("expected still 1 hit after memory reuse, got %d", hits)
    }
}

// When /robots.txt returns 5xx or 401/403 or times out, treat host as
// temporarily disallowed (disallow all) until cache expiry.
func TestMissingRobots_TemporaryDisallow_On5xxAndTimeout(t *testing.T) {
    t.Parallel()
    // 503 case
    var hits503 int32
    srv503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/robots.txt" { http.NotFound(w, r); return }
        atomic.AddInt32(&hits503, 1)
        w.WriteHeader(http.StatusServiceUnavailable)
    }))
    t.Cleanup(srv503.Close)

    ctx := context.Background()
    m1 := &Manager{HTTPClient: srv503.Client(), UserAgent: "goresearch-test", EntryExpiry: time.Minute, AllowPrivateHosts: true}
    u1 := srv503.URL + "/robots.txt"
    rules, src, err := m1.Get(ctx, u1)
    if err != nil {
        t.Fatalf("unexpected error on 5xx policy: %v", err)
    }
    if src != SourceNetwork {
        t.Fatalf("expected SourceNetwork, got %v", src)
    }
    if allowed := rules.IsAllowed("goresearch", "/any"); allowed {
        t.Fatalf("expected disallow-all under temporary disallow (5xx)")
    }
    if atomic.LoadInt32(&hits503) != 1 {
        t.Fatalf("expected 1 hit, got %d", hits503)
    }
    // Memory reuse
    _, src2, err2 := m1.Get(ctx, u1)
    if err2 != nil { t.Fatalf("second get (mem): %v", err2) }
    if src2 != SourceMemory { t.Fatalf("expected SourceMemory, got %v", src2) }
    if atomic.LoadInt32(&hits503) != 1 { t.Fatalf("expected still 1 hit, got %d", hits503) }

    // Timeout case
    var hitsTO int32
    srvTO := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/robots.txt" { http.NotFound(w, r); return }
        atomic.AddInt32(&hitsTO, 1)
        time.Sleep(200 * time.Millisecond)
    }))
    t.Cleanup(srvTO.Close)
    // Clone srv client to add a short timeout
    base := *srvTO.Client()
    base.Timeout = 50 * time.Millisecond
    m2 := &Manager{HTTPClient: &base, UserAgent: "goresearch-test", EntryExpiry: time.Minute, AllowPrivateHosts: true}
    u2 := srvTO.URL + "/robots.txt"
    rulesTO, srcTO, errTO := m2.Get(ctx, u2)
    if errTO != nil {
        t.Fatalf("unexpected error on timeout policy: %v", errTO)
    }
    if srcTO != SourceNetwork { t.Fatalf("expected SourceNetwork, got %v", srcTO) }
    if allowed := rulesTO.IsAllowed("goresearch", "/any"); allowed {
        t.Fatalf("expected disallow-all under temporary disallow (timeout)")
    }
    if atomic.LoadInt32(&hitsTO) != 1 { t.Fatalf("expected 1 hit, got %d", hitsTO) }
    // Memory reuse
    _, srcTO2, errTO2 := m2.Get(ctx, u2)
    if errTO2 != nil { t.Fatalf("second get (mem): %v", errTO2) }
    if srcTO2 != SourceMemory { t.Fatalf("expected SourceMemory, got %v", srcTO2) }
    if atomic.LoadInt32(&hitsTO) != 1 { t.Fatalf("expected still 1 hit, got %d", hitsTO) }
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

// The following tests verify evaluation semantics for robots.txt rules
// as required by FEATURE_CHECKLIST.md (UA precedence, longest-path match,
// Allow vs Disallow precedence, '*' wildcards, and '$' end anchors).

func TestEvaluate_UAPrecedence_AndPathDecisions(t *testing.T) {
    t.Parallel()
    txt := `User-agent: goresearch
Disallow: /private

User-agent: *
Allow: /
`
    rules := parseRobots(txt)

    // Exact UA group should be preferred over wildcard
    if allowed := rules.IsAllowed("goresearch", "/private/page"); allowed {
        t.Fatalf("expected disallow for goresearch on /private/page")
    }
    if allowed := rules.IsAllowed("otheragent", "/private/page"); !allowed {
        t.Fatalf("expected allow for otheragent on /private/page via wildcard allow")
    }

    // Longest-path match with Allow overriding shorter Disallow
    txt2 := `User-agent: goresearch
Disallow: /private
Allow: /private/public
`
    rules2 := parseRobots(txt2)
    if allowed := rules2.IsAllowed("goresearch", "/private/public/info"); !allowed {
        t.Fatalf("expected allow due to longer Allow rule")
    }
    if allowed := rules2.IsAllowed("goresearch", "/private/else"); allowed {
        t.Fatalf("expected disallow for shorter path under disallow")
    }
}

func TestEvaluate_Wildcards_And_Anchors(t *testing.T) {
    t.Parallel()
    txt := `User-agent: goresearch
Disallow: /*.zip$
Allow: /downloads/*.zip$
`
    rules := parseRobots(txt)

    if allowed := rules.IsAllowed("goresearch", "/foo/file.zip"); allowed {
        t.Fatalf("expected disallow for generic *.zip")
    }
    if allowed := rules.IsAllowed("goresearch", "/downloads/file.zip"); !allowed {
        t.Fatalf("expected allow for downloads/*.zip due to longer allow")
    }

    // Query-param pattern example
    txt2 := `User-agent: *
Disallow: /*?session=
`
    rules2 := parseRobots(txt2)
    if allowed := rules2.IsAllowed("any", "/index.html?session=1"); allowed {
        t.Fatalf("expected disallow when pattern with wildcard matches query")
    }
}

func TestEvaluate_CrawlDelayForMatchedGroup(t *testing.T) {
    t.Parallel()
    txt := `User-agent: goresearch
Crawl-delay: 2

User-agent: *
Crawl-delay: 7
`
    rules := parseRobots(txt)
    if d := rules.CrawlDelayFor("goresearch"); d == nil || *d != 2*time.Second {
        t.Fatalf("expected 2s crawl delay for goresearch, got %v", d)
    }
    if d := rules.CrawlDelayFor("other"); d == nil || *d != 7*time.Second {
        t.Fatalf("expected 7s crawl delay for wildcard, got %v", d)
    }
}
