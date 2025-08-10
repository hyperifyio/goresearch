package budget

import (
    "fmt"
    "testing"
)

// Traceability: Implements FEATURE_CHECKLIST.md item "Benchmarks — Add Go benchmarks for fetch, extract, selection, and token budgeting to quantify the impact of concurrency/politeness settings."
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md

func BenchmarkEstimateTokens(b *testing.B) {
	inputs := []int{64, 256, 1024, 4096, 16384, 65536}
	for _, n := range inputs {
        b.Run(sprintf("chars=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = EstimateTokensFromChars(n)
			}
		})
	}
}

func BenchmarkRemainingContext(b *testing.B) {
	cases := []struct{
		name   string
		model  string
		prompt int
		out    int
	}{
		{"gpt-4o 128k, mid prompt", "gpt-4o", 20_000, 1_500},
		{"claude sonnet 200k, large prompt", "claude-3-5-sonnet", 100_000, 2_000},
		{"unknown model default 8k", "mystery-model", 4_000, 1_000},
	}
	for _, cs := range cases {
		b.Run(cs.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = RemainingContextWithHeadroom(cs.model, cs.out, cs.prompt)
			}
		})
	}
}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }