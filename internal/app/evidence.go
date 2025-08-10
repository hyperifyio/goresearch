package app

import (
    "fmt"
    "strings"

    "github.com/hyperifyio/goresearch/internal/verify"
)

// appendEvidenceAppendix returns markdown with an appended Evidence check appendix
// when verification succeeded. On verification error, it returns the input
// markdown unchanged to satisfy the graceful failure contract.
func appendEvidenceAppendix(markdown string, res verify.Result, verifyErr error) string {
    if verifyErr != nil {
        return markdown
    }
    var b strings.Builder
    b.WriteString(markdown)
    // Heading will be labeled by appendix manager later; keep base title here.
    b.WriteString("\n\n## Evidence check\n\n")
    if strings.TrimSpace(res.Summary) != "" {
        b.WriteString(res.Summary)
        b.WriteString("\n\n")
    }
    for i, c := range res.Claims {
        if i >= 20 {
            break
        }
        b.WriteString("- ")
        b.WriteString(strings.TrimSpace(c.Text))
        b.WriteString(" — cites ")
        b.WriteString(formatCitations(c.Citations))
        b.WriteString("; confidence: ")
        b.WriteString(c.Confidence)
        b.WriteString("; supported: ")
        if c.Supported {
            b.WriteString("true")
        } else {
            b.WriteString("false")
        }
        b.WriteString("\n")
    }
    return b.String()
}

func formatCitations(cites []int) string {
    if len(cites) == 0 {
        return "[]"
    }
    parts := make([]string, len(cites))
    for i, n := range cites {
        parts[i] = fmt.Sprintf("%d", n)
    }
    return "[" + strings.Join(parts, ",") + "]"
}


