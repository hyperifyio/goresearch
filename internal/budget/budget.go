package budget

import (
    "math"
    "regexp"
    "strings"
)

// EstimateTokensFromChars converts a character count into an estimated token
// count using a conservative heuristic (~4 chars per token in English). The
// result is always at least 1 when chars > 0.
func EstimateTokensFromChars(charCount int) int {
    if charCount <= 0 {
        return 0
    }
    // Keep conservative to avoid overruns. Use ceiling for safety.
    return int(math.Ceil(float64(charCount) / 4.0))
}

// EstimateTokens returns the estimated token count of a string.
func EstimateTokens(s string) int {
    return EstimateTokensFromChars(len(s))
}

// EstimatePromptTokens estimates the total tokens for a prompt composed of
// a system message, a user message, and zero or more excerpts.
func EstimatePromptTokens(system string, user string, excerpts []string) int {
    total := 0
    total += EstimateTokens(system)
    total += EstimateTokens(user)
    for _, ex := range excerpts {
        total += EstimateTokens(ex)
    }
    return total
}

// ModelContextTokens returns an estimated maximum context window for a given
// model name. Unknown models fall back to a sensible default.
func ModelContextTokens(modelName string) int {
    name := strings.ToLower(strings.TrimSpace(modelName))
    if name == "" {
        return 8192
    }
    if v, ok := knownModelMax[name]; ok {
        return v
    }
    // Heuristics based on common suffixes present in model names
    if containsNumberSuffix(name, "1m") {
        return 1_000_000
    }
    if containsNumberSuffix(name, "512k") {
        return 512_000
    }
    if containsNumberSuffix(name, "200k") {
        return 200_000
    }
    if containsNumberSuffix(name, "180k") {
        return 180_000
    }
    if containsNumberSuffix(name, "128k") {
        return 128_000
    }
    if strings.Contains(name, "-mini") {
        // Many "mini" models expose large contexts nowadays, assume 128k.
        return 128_000
    }
    // Default conservative context if unknown.
    return 8192
}

// RemainingContext computes the remaining input token budget given a model,
// a desired reservation for output generation, and the estimated prompt tokens.
// The result is never negative.
func RemainingContext(modelName string, reservedForOutput int, promptTokens int) int {
    maxCtx := ModelContextTokens(modelName)
    if reservedForOutput < 0 {
        reservedForOutput = 0
    }
    remaining := maxCtx - reservedForOutput - promptTokens
    if remaining < 0 {
        return 0
    }
    return remaining
}

// FitsInContext reports whether the prompt can fit into the model's context
// window when reserving the specified number of output tokens.
func FitsInContext(modelName string, reservedForOutput int, promptTokens int) bool {
    return RemainingContext(modelName, reservedForOutput, promptTokens) > 0
}

// knownModelMax contains rough context sizes for common model identifiers.
// These are best-effort and do not need to be exhaustive.
var knownModelMax = map[string]int{
    // OpenAI family (approximate)
    "gpt-4o":            128_000,
    "gpt-4o-mini":       128_000,
    "gpt-4-turbo":       128_000,
    "gpt-4-0125-preview": 128_000,
    "gpt-3.5-turbo":     16_384,

    // Anthropic (approximate)
    "claude-3-5-sonnet": 200_000,
    "claude-3-opus":     200_000,
    "claude-3-sonnet":   200_000,
    "claude-3-haiku":    200_000,

    // Llama and other popular OSS defaults (high variance in practice)
    "llama-3":           8_192,
    "llama-3.1":         128_000,
}

var suffixRe = regexp.MustCompile(`(?i)(\d+)(k|m)$`)

func containsNumberSuffix(s string, suffix string) bool {
    return strings.HasSuffix(s, strings.ToLower(suffix))
}


