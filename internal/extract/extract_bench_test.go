package extract

import (
    "strings"
    "testing"
)

// Traceability: Implements FEATURE_CHECKLIST.md item "Benchmarks — Add Go benchmarks for fetch, extract, selection, and token budgeting to quantify the impact of concurrency/politeness settings."
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md

// Benchmark FromHTML on representative HTML sizes and structures.
func BenchmarkFromHTML(b *testing.B) {
	small := []byte("<html><head><title>t</title></head><body><main><p>a</p></main></body></html>")
	medium := makeHTML(50, 60)
	large := makeHTML(200, 200)

	b.Run("small", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = FromHTML(small)
		}
	})
	b.Run("medium", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = FromHTML(medium)
		}
	})
	b.Run("large", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = FromHTML(large)
		}
	})
}

func makeHTML(paras int, itemsPerList int) []byte {
    builder := new(strings.Builder)
	builder.WriteString("<html><head><title>demo</title></head><body><main>")
	for i := 0; i < paras; i++ {
		builder.WriteString("<h2>Heading</h2><p>")
		builder.WriteString(sampleText)
		builder.WriteString("</p>")
	}
	builder.WriteString("<ul>")
	for i := 0; i < itemsPerList; i++ {
		builder.WriteString("<li>")
		builder.WriteString(sampleText)
		builder.WriteString("</li>")
	}
	builder.WriteString("</ul></main></body></html>")
	return []byte(builder.String())
}

const sampleText = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."