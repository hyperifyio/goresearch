package app

import (
	"strings"
	"testing"
	"time"

	"github.com/hyperifyio/goresearch/internal/synth"
)

func TestBuildManifestEntriesFromSynth_ComputesSHA256AndChars(t *testing.T) {
	ex := []synth.SourceExcerpt{
		{Index: 1, URL: "https://example.com/a", Title: "A", Excerpt: "hello"},
		{Index: 2, URL: "https://example.com/b", Title: "B", Excerpt: "world\n"},
	}
	entries := buildManifestEntriesFromSynth(ex)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries; got %d", len(entries))
	}
	if entries[0].Chars != 5 || entries[1].Chars != 5 {
		t.Fatalf("unexpected char counts: %+v", entries)
	}
	if entries[0].SHA256 == "" || entries[1].SHA256 == "" {
		t.Fatalf("expected non-empty digests")
	}
}

func TestAppendEmbeddedManifest_AppendsReadableSection(t *testing.T) {
	base := "# Doc\n\nBody\n"
	meta := manifestMeta{
		Model:       "gpt-local",
		LLMBaseURL:  "http://localhost:11434/v1",
		SourceCount: 2,
		HTTPCache:   true,
		LLMCache:    true,
		GeneratedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	entries := []manifestEntry{{Index: 1, URL: "https://example.com/a", SHA256: "abcd", Chars: 5}}
	out := appendEmbeddedManifest(base, meta, entries)
	if !strings.Contains(out, "## Manifest") {
		t.Fatalf("expected a Manifest section")
	}
	if !strings.Contains(out, "gpt-local") || !strings.Contains(out, "http://localhost:11434/v1") {
		t.Fatalf("expected header fields present")
	}
	if !strings.Contains(out, "1. https://example.com/a — sha256=abcd; chars=5") {
		t.Fatalf("expected entry line; got:\n%s", out)
	}
}

func TestAppendEmbeddedManifestWithSkipped_AppendsSkippedSection(t *testing.T) {
    base := "# Doc\n\nBody\n"
    meta := manifestMeta{Model: "gpt-local", LLMBaseURL: "http://localhost:11434/v1", SourceCount: 1, HTTPCache: true, LLMCache: true, GeneratedAt: time.Date(2024,1,1,0,0,0,0,time.UTC)}
    entries := []manifestEntry{{Index: 1, URL: "https://example.com", SHA256: "abcd", Chars: 4}}
    skipped := []skippedEntry{{URL: "https://example.org/blocked", Reason: "X-Robots-Tag:noai"}}
    out := appendEmbeddedManifestWithSkipped(base, meta, entries, skipped)
    if !strings.Contains(out, "### Skipped due to robots/opt-out") {
        t.Fatalf("expected skipped section header")
    }
    if !strings.Contains(out, "https://example.org/blocked — X-Robots-Tag:noai") {
        t.Fatalf("expected skipped entry line; got:\n%s", out)
    }
}
