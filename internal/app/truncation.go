package app

import (
    "fmt"
    "math"
    "strings"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/budget"
    "github.com/hyperifyio/goresearch/internal/synth"
)

// proportionallyTruncateExcerpts enforces a global budget by scaling each
// excerpt length proportionally so that the combined prompt fits within the
// model context when reserving output tokens. It preserves ordering and keeps
// every selected source, avoiding hard drops.
func proportionallyTruncateExcerpts(b brief.Brief, outline []string, in []synth.SourceExcerpt, cfg Config) []synth.SourceExcerpt {
    if len(in) == 0 {
        return in
    }

    // 1) Establish conservative baseline token cost excluding excerpt bodies.
    // Keep consistent with estimateSynthesisBudget.
    const systemChars = 800

    var userBuilder strings.Builder
    userBuilder.WriteString("Write a single cohesive Markdown document with:")
    userBuilder.WriteString("\n- A title on the first line")
    userBuilder.WriteString("\n- A date below the title in ISO format (YYYY-MM-DD)")
    userBuilder.WriteString("\n- An executive summary")
    if len(outline) > 0 {
        userBuilder.WriteString("\n- Body sections matching this outline, in order:")
        for _, h := range outline {
            userBuilder.WriteString("\n  - ")
            userBuilder.WriteString(h)
        }
    }
    userBuilder.WriteString("\n- A 'Risks and limitations' section")
    userBuilder.WriteString("\n- A 'References' section listing all sources as a numbered list with titles and full URLs")
    userBuilder.WriteString("\n- An 'Evidence check' appendix summarizing key claims with supporting source indices and confidence")
    if strings.TrimSpace(cfg.LanguageHint) != "" {
        userBuilder.WriteString("\nWrite in language: ")
        userBuilder.WriteString(cfg.LanguageHint)
    }
    userBuilder.WriteString("\n\nBrief topic: ")
    userBuilder.WriteString(b.Topic)
    if b.AudienceHint != "" {
        userBuilder.WriteString("\nAudience: ")
        userBuilder.WriteString(b.AudienceHint)
    }
    if b.ToneHint != "" {
        userBuilder.WriteString("\nTone: ")
        userBuilder.WriteString(b.ToneHint)
    }
    if b.TargetLengthWords > 0 {
        userBuilder.WriteString("\nTarget length: ")
        userBuilder.WriteString(fmt.Sprintf("%d words", b.TargetLengthWords))
    }
    userBuilder.WriteString("\n\nSources (use only these; cite with [n]):\n")
    for _, src := range in {
        // Header lines per source without excerpt body
        userBuilder.WriteString(fmt.Sprintf("%d. %s — %s\n", src.Index, src.Title, src.URL))
        // We will include the literal word "Excerpt:" label in baseline since
        // it appears even when bodies are empty in our prompt contract.
        userBuilder.WriteString("Excerpt:\n\n")
    }
    baselineTokens := budget.EstimateTokensFromChars(systemChars + userBuilder.Len())

    // 2) Determine available token budget for excerpt bodies.
    reserved := cfg.ReservedOutputTokens
    if reserved <= 0 {
        reserved = 1500
    }
    maxCtx := budget.ModelContextTokens(cfg.LLMModel)
    availableForExcerptsTokens := maxCtx - reserved - baselineTokens
    if availableForExcerptsTokens <= 0 {
        // No room for excerpts; keep headers only.
        out := make([]synth.SourceExcerpt, 0, len(in))
        for _, src := range in {
            out = append(out, synth.SourceExcerpt{Index: src.Index, Title: src.Title, URL: src.URL, Excerpt: ""})
        }
        return out
    }

    // 3) Compute current excerpt token estimate.
    currentExcerptTokens := 0
    for _, src := range in {
        currentExcerptTokens += budget.EstimateTokens(src.Excerpt)
    }
    if currentExcerptTokens <= availableForExcerptsTokens {
        // Already fits; no change.
        return in
    }

    // 4) Scale each excerpt proportionally based on token budget.
    scale := float64(availableForExcerptsTokens) / float64(currentExcerptTokens)
    if scale < 0 {
        scale = 0
    }
    out := make([]synth.SourceExcerpt, 0, len(in))
    for _, src := range in {
        if strings.TrimSpace(src.Excerpt) == "" || scale == 0 {
            out = append(out, synth.SourceExcerpt{Index: src.Index, Title: src.Title, URL: src.URL, Excerpt: ""})
            continue
        }
        // Target bytes proportional to original length. Use floor for safety.
        targetBytes := int(math.Floor(float64(len(src.Excerpt)) * scale))
        if targetBytes <= 0 {
            // Keep a tiny stub to avoid dropping the source entirely.
            targetBytes = 0
        }
        truncated := trimByByteLimitPreservingRunes(src.Excerpt, targetBytes)
        out = append(out, synth.SourceExcerpt{Index: src.Index, Title: src.Title, URL: src.URL, Excerpt: truncated})
    }

    return out
}

// trimByByteLimitPreservingRunes returns a prefix of s whose byte length is
// <= maxBytes, never splitting a UTF-8 rune. If maxBytes >= len(s) it returns s.
func trimByByteLimitPreservingRunes(s string, maxBytes int) string {
    if maxBytes >= len(s) {
        return s
    }
    if maxBytes <= 0 || len(s) == 0 {
        return ""
    }
    // Walk runes, track byte indices, and stop before exceeding limit.
    // This is O(n) over runes; acceptable for excerpt trimming.
    var idx int
    for i := range s {
        if i > maxBytes {
            break
        }
        idx = i
    }
    // idx is the last rune boundary <= maxBytes; ensure non-negative.
    if idx < 0 {
        return ""
    }
    // If idx is 0 and maxBytes < first rune boundary, return empty.
    if idx == 0 && maxBytes < len(s) {
        return ""
    }
    return s[:idx]
}


