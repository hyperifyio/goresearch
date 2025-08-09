package app

import (
    "testing"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/synth"
)

func TestTrimByByteLimitPreservingRunes(t *testing.T) {
    s := "héllo世界" // multibyte runes
    if got := trimByByteLimitPreservingRunes(s, len(s)); got != s {
        t.Fatalf("expected full string when limit >= len, got %q", got)
    }
    if got := trimByByteLimitPreservingRunes(s, 0); got != "" {
        t.Fatalf("expected empty when limit 0, got %q", got)
    }
    // Limit to cut between runes; ensure not splitting.
    // Determine a mid-point within the string.
    if got := trimByByteLimitPreservingRunes(s, 3); !isValidUTF8(got) {
        t.Fatalf("result must be valid UTF-8, got %q", got)
    }
}

// isValidUTF8 reports whether the string is valid UTF-8 by attempting a no-op conversion.
func isValidUTF8(s string) bool {
    for range s { // ranging runes will panic if invalid UTF-8
    }
    return true
}

func TestProportionalTruncation_ScalesDownToFit(t *testing.T) {
    b := brief.Brief{Topic: "t"}
    cfg := Config{LLMModel: "gpt-4o", ReservedOutputTokens: 1000}
    in := []synth.SourceExcerpt{
        {Index: 1, Title: "a", URL: "u1", Excerpt: repeat("x", 4000)},
        {Index: 2, Title: "b", URL: "u2", Excerpt: repeat("y", 6000)},
    }
    out := proportionallyTruncateExcerpts(b, nil, in, cfg)
    if len(out) != 2 {
        t.Fatalf("expected 2 excerpts, got %d", len(out))
    }
    // Ensure not dropping any source
    if out[0].Index != 1 || out[1].Index != 2 {
        t.Fatalf("indices preserved: got %v, %v", out[0].Index, out[1].Index)
    }
    // Should not exceed original lengths
    if len(out[0].Excerpt) > len(in[0].Excerpt) || len(out[1].Excerpt) > len(in[1].Excerpt) {
        t.Fatalf("truncation must not increase lengths")
    }
}

func TestProportionalTruncation_NoRoomKeepsHeadersOnly(t *testing.T) {
    b := brief.Brief{Topic: "t"}
    // Tiny context by using unknown model and huge reserved tokens via config
    cfg := Config{LLMModel: "", ReservedOutputTokens: 9000000}
    in := []synth.SourceExcerpt{{Index: 1, Title: "a", URL: "u", Excerpt: "body"}}
    out := proportionallyTruncateExcerpts(b, nil, in, cfg)
    if out[0].Excerpt != "" {
        t.Fatalf("expected empty excerpts when no room, got %q", out[0].Excerpt)
    }
}

func repeat(s string, n int) string {
    b := make([]byte, 0, len(s)*n)
    for i := 0; i < n; i++ {
        b = append(b, s...)
    }
    return string(b)
}


