package aggregate

import (
	"testing"

	"github.com/hyperifyio/goresearch/internal/search"
)

func TestMergeAndNormalize_Dedup_TrimUTM(t *testing.T) {
	groups := [][]search.Result{
		{
			{Title: "A", URL: "https://example.com/page?utm_source=x&utm_medium=y", Snippet: "one"},
		},
		{
			{Title: "A dup", URL: "https://EXAMPLE.com/page", Snippet: "two"},
		},
	}
	out := MergeAndNormalize(groups)
	if len(out) != 1 {
		t.Fatalf("expected 1 after dedup, got %d", len(out))
	}
	if out[0].URL != "https://example.com/page" {
		t.Fatalf("unexpected normalized url: %q", out[0].URL)
	}
}
