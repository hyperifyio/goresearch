package fetch

import (
    "context"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strconv"
    "strings"
    "sync/atomic"
    "testing"
    "time"

    "github.com/hyperifyio/goresearch/internal/robots"
)

// Traceability: Implements FEATURE_CHECKLIST.md item "Benchmarks — Add Go benchmarks for fetch, extract, selection, and token budgeting to quantify the impact of concurrency/politeness settings."
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md

// Benchmark the fetch.Client under different concurrency and robots crawl-delay settings.
func BenchmarkClient_FetchConcurrencyAndPoliteness(b *testing.B) {
	// Test HTTP server that serves a small HTML page and a configurable robots.txt
	var crawlDelayAtomic int64 // nanoseconds; updated by sub-benchmarks
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		d := time.Duration(atomic.LoadInt64(&crawlDelayAtomic))
		// Emit Crawl-delay in seconds with fractional precision when small
		sec := float64(d) / float64(time.Second)
		w.Header().Set("Content-Type", "text/plain")
		if sec <= 0 {
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}
		// Format with up to 3 decimals to keep tiny delays
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\nCrawl-delay: "))
		_, _ = w.Write([]byte(strings.TrimRight(strings.TrimRight(sprintfFloat(sec, 3), "0"), ".")))
		_, _ = w.Write([]byte("\n"))
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><head><title>ok</title></head><body><main><p>hello</p></main></body></html>"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Helper to run a scenario
	runScenario := func(name string, maxConc int, useRobots bool, crawlDelay time.Duration) {
		b.Run(name, func(b *testing.B) {
			atomic.StoreInt64(&crawlDelayAtomic, int64(crawlDelay))
			var mgr *robots.Manager
			if useRobots {
				mgr = &robots.Manager{HTTPClient: ts.Client(), UserAgent: "bench/1", EntryExpiry: time.Hour, AllowPrivateHosts: true}
			}
			cli := &Client{
				HTTPClient:       ts.Client(),
				UserAgent:        "bench/1",
				MaxAttempts:      1,
				PerRequestTimeout: 2 * time.Second,
				MaxConcurrent:    maxConc,
				AllowPrivateHosts: true,
				Robots:           mgr,
			}
			url := ts.URL + "/page"
			// Warm robots to avoid counting its network fetch in the timed section
			if useRobots {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, _, _ = mgr.Get(ctx, ts.URL+"/robots.txt")
				cancel()
			}
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_, _, err := cli.Get(ctx, url)
					cancel()
					if err != nil {
						b.Fatalf("fetch failed: %v", err)
					}
				}
			})
		})
	}

	runScenario("conc=1,no-robots", 1, false, 0)
	runScenario("conc=8,no-robots", 8, false, 0)
	runScenario("conc=8,robots-delay=0s", 8, true, 0)
	// Small crawl-delay to keep benchmark fast while exercising scheduler
	runScenario("conc=8,robots-delay=2ms", 8, true, 2*time.Millisecond)
}

// sprintfFloat formats f with up to prec decimals, trimming trailing zeros when caller desires.
func sprintfFloat(f float64, prec int) string {
    fmtStr := "%0." + strconv.Itoa(prec) + "f"
    return sprintf(fmtStr, f)
}

// Small wrappers avoid pulling fmt into hot loop code paths above.
func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }