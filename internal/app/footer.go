package app

import (
    "fmt"
    "strconv"
    "strings"
)

// appendReproFooter appends a minimal, deterministic footer that records
// configuration useful for reproducibility and auditing.
// The footer includes: model name, LLM base URL, number of sources used,
// and whether HTTP and LLM caching were active.
func appendReproFooter(markdown string, model string, baseURL string, numSources int, httpCacheActive bool, llmCacheActive bool) string {
    var b strings.Builder
    b.WriteString(markdown)
    b.WriteString("\n\n---\n")
    b.WriteString("Reproducibility: ")
    b.WriteString("model=")
    b.WriteString(strings.TrimSpace(model))
    b.WriteString("; llm_base_url=")
    b.WriteString(strings.TrimSpace(baseURL))
    b.WriteString("; sources_used=")
    b.WriteString(strconv.Itoa(numSources))
    b.WriteString("; http_cache=")
    b.WriteString(fmt.Sprintf("%t", httpCacheActive))
    b.WriteString("; llm_cache=")
    b.WriteString(fmt.Sprintf("%t", llmCacheActive))
    b.WriteString("\n")
    return b.String()
}


