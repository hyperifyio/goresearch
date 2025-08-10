package app

import (
    "regexp"
    "strings"
    "time"
)

// enrichReferences scans the Markdown references section and applies deterministic
// enrichments:
// - Convert certain URLs to stable/canonical forms (arXiv abs, RFC Editor)
// - If a DOI is detectable in the line, ensure a canonical DOI URL is present
// - Append "(Accessed on YYYY-MM-DD)" for web sources lacking an access date
// The function is conservative: it only rewrites within the References block and
// keeps other content unchanged. It requires no network access.
func enrichReferences(markdown string, now func() time.Time) string {
    if now == nil { now = time.Now }
    lines := strings.Split(markdown, "\n")
    inRefs := false
    order := 0

    // Regexes
    headingRe := regexp.MustCompile(`^#{1,6}\s+References\s*$`)
    numItemRe := regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
    urlRe := regexp.MustCompile(`https?://[^\s)]+`)
    accessedRe := regexp.MustCompile(`(?i)accessed on\s+\d{4}-\d{2}-\d{2}`)
    // DOI core pattern (per Crossref guidance, simplified and case-insensitive)
    doiCore := `10\.[0-9]{4,9}/[-._;()/:A-Za-z0-9]+`
    doiTextRe := regexp.MustCompile(`(?i)(?:doi\s*[:]?\s*|https?://(?:dx\.)?doi\.org/)(` + doiCore + `)`) // capture DOI

    for i := 0; i < len(lines); i++ {
        s := strings.TrimSpace(lines[i])
        if s == "" { continue }
        if headingRe.MatchString(s) {
            inRefs = true
            order = 0
            continue
        }
        if inRefs {
            // Stop at next heading
            if strings.HasPrefix(s, "#") {
                inRefs = false
                continue
            }
            // Process numbered list items only
            m := numItemRe.FindStringSubmatch(s)
            if m == nil { continue }
            order++
            content := strings.TrimSpace(m[2])

            // Identify first URL
            loc := urlRe.FindStringIndex(content)
            if loc != nil {
                url := content[loc[0]:loc[1]]
                stable := toStableURL(url)
                if stable != url {
                    // Replace only that occurrence
                    content = content[:loc[0]] + stable + content[loc[1]:]
                }
            }

            // Ensure DOI URL included when DOI detectable
            if dm := doiTextRe.FindStringSubmatch(content); dm != nil && len(dm) >= 2 {
                doi := dm[1]
                doiURL := "https://doi.org/" + doi
                if !strings.Contains(strings.ToLower(content), strings.ToLower(doiURL)) {
                    // Append as " DOI: https://doi.org/<doi>"
                    if strings.HasSuffix(content, ".") { content = strings.TrimSuffix(content, ".") }
                    content = content + " DOI: " + doiURL
                }
            }

            // Append Accessed on date for web sources without one
            // Heuristic: if any http(s) URL exists on the line
            if urlRe.FindStringIndex(content) != nil && !accessedRe.MatchString(content) {
                ymd := now().UTC().Format("2006-01-02")
                // Canonical form: " (Accessed on YYYY-MM-DD)"
                content = content + " (Accessed on " + ymd + ")"
            }

            // Rewrite the line with preserved leading number
            lines[i] = m[1] + ". " + content
        }
    }
    return strings.Join(lines, "\n")
}

// toStableURL converts certain known URLs to stable forms without network access.
// - arXiv PDF -> arXiv abs
// - IETF datatracker RFC -> rfc-editor canonical RFC URL
func toStableURL(u string) string {
    lower := strings.ToLower(u)
    // arXiv: prefer abs over pdf
    if strings.HasPrefix(lower, "https://arxiv.org/pdf/") && strings.HasSuffix(lower, ".pdf") {
        // https://arxiv.org/pdf/1234.56789.pdf -> https://arxiv.org/abs/1234.56789
        core := strings.TrimSuffix(u[len("https://arxiv.org/pdf/"):], ".pdf")
        return "https://arxiv.org/abs/" + core
    }
    // IETF datatracker HTML for RFC -> RFC Editor canonical
    // https://datatracker.ietf.org/doc/html/rfc9110 -> https://www.rfc-editor.org/rfc/rfc9110
    if strings.HasPrefix(lower, "https://datatracker.ietf.org/doc/html/rfc") {
        idx := strings.LastIndex(lower, "/rfc")
        if idx >= 0 {
            tail := u[idx+1:] // rfcNNNN
            return "https://www.rfc-editor.org/rfc/" + tail
        }
    }
    return u
}
