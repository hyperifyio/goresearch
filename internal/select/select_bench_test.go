package selecter

import (
    "fmt"
    "math/rand"
    "testing"

    "github.com/hyperifyio/goresearch/internal/search"
)

// Traceability: Implements FEATURE_CHECKLIST.md item "Benchmarks — Add Go benchmarks for fetch, extract, selection, and token budgeting to quantify the impact of concurrency/politeness settings."
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md

func BenchmarkSelect(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	makeResults := func(n int) []search.Result {
		out := make([]search.Result, n)
		for i := 0; i < n; i++ {
			// Random hosts across a modest set to exercise per-domain caps
			hostIdx := rng.Intn(20)
			out[i] = search.Result{
				Title:   sprintf("T %d", i),
				URL:     sprintf("https://host%02d.example.com/path/%d?q=x", hostIdx, i),
				Snippet: randSnippet(rng, 20, 200),
				Source:  "bench",
			}
		}
		return out
	}

	cases := []struct{
		name string
		n    int
		opt  Options
	}{
		{"n=50, default", 50, Options{}},
		{"n=200, default", 200, Options{}},
		{"n=200, preferPrimary", 200, Options{PreferPrimary: true}},
		{"n=200, language=en", 200, Options{PreferredLanguage: "en"}},
	}

	for _, cs := range cases {
		b.Run(cs.name, func(b *testing.B) {
			res := makeResults(cs.n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = Select(res, cs.opt)
			}
		})
	}
}

func randSnippet(rng *rand.Rand, min, max int) string {
	n := rng.Intn(max-min+1) + min
	buf := make([]byte, 0, n)
	for len(buf) < n {
		buf = append(buf, sampleSnippet...)
	}
	return string(buf[:n])
}

const sampleSnippet = "This is a sample snippet with a variety of common English words to trigger detection and ranking. "

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }