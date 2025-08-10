package app

import (
    "fmt"
    "testing"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/search"
)

// Traceability: Implements FEATURE_CHECKLIST.md item "Benchmarks — Add Go benchmarks for fetch, extract, selection, and token budgeting to quantify the impact of concurrency/politeness settings."
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md

func BenchmarkEstimateSynthesisBudget(b *testing.B) {
	br := brief.Brief{Topic: "Test topic", AudienceHint: "engineers", ToneHint: "concise", TargetLengthWords: 1200}
	outline := []string{"Intro", "Body", "Conclusion"}
	selected := make([]search.Result, 200)
	for i := range selected {
		selected[i] = search.Result{Title: sprintf("T %d", i), URL: sprintf("https://example.com/%d", i)}
	}
	cfg := Config{LLMModel: "gpt-4o", PerSourceChars: 8000}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = estimateSynthesisBudget(br, outline, selected, cfg)
	}
}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }