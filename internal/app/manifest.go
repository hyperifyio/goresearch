package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/hyperifyio/goresearch/internal/synth"
)

// manifestEntry is a compact record of a single source used in synthesis.
type manifestEntry struct {
	Index  int    `json:"index"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	SHA256 string `json:"sha256"`
	Chars  int    `json:"chars"`
}

// manifestMeta captures high-level run details that aid reproducibility.
type manifestMeta struct {
	Model       string    `json:"model"`
	LLMBaseURL  string    `json:"llm_base_url"`
	SourceCount int       `json:"source_count"`
	HTTPCache   bool      `json:"http_cache"`
	LLMCache    bool      `json:"llm_cache"`
	GeneratedAt time.Time `json:"generated_at"`
}

// computeSHA256Hex returns a lowercase hex-encoded SHA-256 of the given text.
func computeSHA256Hex(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// buildManifestEntriesFromSynth constructs entries from the final excerpts used for synthesis.
func buildManifestEntriesFromSynth(excerpts []synth.SourceExcerpt) []manifestEntry {
	out := make([]manifestEntry, 0, len(excerpts))
	for _, e := range excerpts {
		content := strings.TrimSpace(e.Excerpt)
		out = append(out, manifestEntry{
			Index:  e.Index,
			URL:    strings.TrimSpace(e.URL),
			Title:  strings.TrimSpace(e.Title),
			SHA256: computeSHA256Hex(content),
			Chars:  len(content),
		})
	}
	return out
}

// appendEmbeddedManifest appends a compact Markdown manifest section listing
// canonical URLs and the digest of the exact content used for synthesis.
func appendEmbeddedManifest(markdown string, meta manifestMeta, entries []manifestEntry) string {
	var b strings.Builder
	b.WriteString(markdown)
	b.WriteString("\n\n## Manifest\n\n")
	// Minimal, readable header
	b.WriteString("- Model: ")
	b.WriteString(strings.TrimSpace(meta.Model))
	b.WriteString("\n- LLM base URL: ")
	b.WriteString(strings.TrimSpace(meta.LLMBaseURL))
	b.WriteString("\n- Sources: ")
	b.WriteString(strconv.Itoa(meta.SourceCount))
	b.WriteString("\n- HTTP cache: ")
	b.WriteString(boolToString(meta.HTTPCache))
	b.WriteString("\n- LLM cache: ")
	b.WriteString(boolToString(meta.LLMCache))
	b.WriteString("\n- Generated: ")
	b.WriteString(meta.GeneratedAt.UTC().Format(time.RFC3339))
	b.WriteString("\n\n")

	// List sources with stable numbering and digests
	for _, e := range entries {
		b.WriteString(strconv.Itoa(e.Index))
		b.WriteString(". ")
		b.WriteString(e.URL)
		b.WriteString(" — sha256=")
		b.WriteString(e.SHA256)
		b.WriteString("; chars=")
		b.WriteString(strconv.Itoa(e.Chars))
		b.WriteString("\n")
	}
	return b.String()
}

// marshalManifestJSON encodes a machine-readable sidecar manifest.
func marshalManifestJSON(meta manifestMeta, entries []manifestEntry) ([]byte, error) {
	payload := struct {
		Meta    manifestMeta    `json:"meta"`
		Sources []manifestEntry `json:"sources"`
	}{Meta: meta, Sources: entries}
	return json.MarshalIndent(payload, "", "  ")
}

// deriveManifestSidecarPath returns a sidecar JSON path next to the output Markdown.
func deriveManifestSidecarPath(outputPath string) string {
	return outputPath + ".manifest.json"
}

func boolToString(bv bool) string {
	if bv {
		return "true"
	}
	return "false"
}
