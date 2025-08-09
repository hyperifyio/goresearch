package app

import (
    "fmt"
    "math"
    "strings"

    "github.com/hyperifyio/goresearch/internal/budget"
    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/search"
)

// BudgetEstimate provides a simple view of prompt sizing and remaining headroom.
type BudgetEstimate struct {
    ModelContext   int
    PromptTokens   int
    ReservedOutput int
    Remaining      int
    Fits           bool
}

// estimateSynthesisBudget computes a conservative estimate for the synthesis prompt.
func estimateSynthesisBudget(b brief.Brief, outline []string, selected []search.Result, cfg Config) BudgetEstimate {
    // Rough constant for the system prompt used in synthesis. Keep conservative.
    const systemChars = 800

    // Build an approximate user message that would be sent to the synthesizer.
    var userBuilder strings.Builder
    userBuilder.WriteString("Brief topic: ")
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
    if len(outline) > 0 {
        userBuilder.WriteString("\nOutline:\n")
        for _, h := range outline {
            userBuilder.WriteString("- ")
            userBuilder.WriteString(h)
            userBuilder.WriteString("\n")
        }
    }
    if len(selected) > 0 {
        userBuilder.WriteString("\nSources:\n")
        for i, r := range selected {
            userBuilder.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, r.Title, r.URL))
        }
    }
    userChars := userBuilder.Len()

    // Estimate excerpts budget as the configured per-source cap times count.
    per := cfg.PerSourceChars
    if per <= 0 {
        per = 12_000
    }
    excerptsChars := per * len(selected)

    // Tokenize conservatively.
    promptTokens := budget.EstimateTokensFromChars(systemChars + userChars + excerptsChars)

    // Reserve output tokens based on requested word length; keep conservative multiplier.
    words := b.TargetLengthWords
    if words <= 0 {
        words = 1200
    }
    reserved := int(math.Ceil(float64(words) * 1.4))

    remaining := budget.RemainingContext(cfg.LLMModel, reserved, promptTokens)
    return BudgetEstimate{
        ModelContext:   budget.ModelContextTokens(cfg.LLMModel),
        PromptTokens:   promptTokens,
        ReservedOutput: reserved,
        Remaining:      remaining,
        Fits:           remaining > 0,
    }
}


