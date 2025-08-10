package app

import (
    "strings"
)

// appendAutoToC inserts a Markdown Table of contents after the header/metadata
// block when the document contains at least minHeadings headings (excluding the
// H1 title). If a ToC already exists, the document is returned unchanged.
// Headings considered for the ToC are H2–H4; References/Glossary/Evidence/Manifest
// sections are excluded from the ToC by name.
func appendAutoToC(markdown string, minHeadings int) string {
    if minHeadings <= 0 { minHeadings = 12 }
    if containsHeadingCase(markdown, "table of contents") {
        return markdown
    }
    lines := strings.Split(markdown, "\n")

    // Collect headings and their levels (skip the first H1 title)
    type item struct{ level int; text string }
    items := make([]item, 0, 64)
    h1Seen := false
    for _, raw := range lines {
        s := strings.TrimSpace(raw)
        if !strings.HasPrefix(s, "#") { continue }
        level := countPrefix(s, '#')
        if level < 1 || level > 6 { continue }
        // Extract text after heading marks and optional space
        t := strings.TrimSpace(strings.TrimLeft(s, "#"))
        if t == "" { continue }
        if !h1Seen && level == 1 {
            h1Seen = true
            continue // Do not include H1 in ToC
        }
        if level >= 2 && level <= 4 {
            // Exclude common appendix/meta sections from ToC
            tl := strings.ToLower(t)
            if tl == "references" || tl == "glossary" || strings.HasPrefix(tl, "evidence") || strings.Contains(tl, "manifest") {
                continue
            }
            items = append(items, item{level: level, text: t})
        }
    }
    if len(items) < minHeadings { return markdown }

    // Build ToC block
    var b strings.Builder
    b.WriteString("## Table of contents\n\n")
    for _, it := range items {
        indent := ""
        if it.level == 3 { indent = "  " }
        if it.level == 4 { indent = "    " }
        slug := makeSlugForToC(it.text)
        if slug == "" { continue }
        b.WriteString(indent)
        b.WriteString("- [")
        b.WriteString(it.text)
        b.WriteString("](#")
        b.WriteString(slug)
        b.WriteString(")\n")
    }
    b.WriteString("\n")

    // Determine insertion point: after title, date, and early metadata
    insertAt := indexAfterHeaderAndMetadata(lines)

    out := make([]string, 0, len(lines)+16)
    out = append(out, lines[:insertAt]...)
    // Ensure a blank line before ToC
    if insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) != "" {
        out = append(out, "")
    }
    out = append(out, b.String())
    // Ensure a blank line after ToC
    if insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) != "" {
        out = append(out, "")
    }
    out = append(out, lines[insertAt:]...)
    return strings.Join(out, "\n")
}

// indexAfterHeaderAndMetadata returns the line index after the initial header
// block consisting of: first H1, next non-empty line (date), and up to ~40 lines
// scanning for Author:/Version: metadata. Falls back to after the first H1.
func indexAfterHeaderAndMetadata(lines []string) int {
    // Find first H1
    first := -1
    for i, raw := range lines {
        s := strings.TrimSpace(raw)
        if strings.HasPrefix(s, "# ") { first = i; break }
        if s != "" { break }
    }
    if first == -1 { return 0 }
    // Find second non-empty (expected date)
    second := -1
    for i := first + 1; i < len(lines); i++ {
        if strings.TrimSpace(lines[i]) == "" { continue }
        second = i; break
    }
    idx := first + 1
    if second != -1 { idx = second + 1 }
    // Scan limited window for metadata lines and advance past them
    limit := idx + 40
    if limit > len(lines) { limit = len(lines) }
    for i := idx; i < limit; i++ {
        s := strings.TrimSpace(lines[i])
        if s == "" { continue }
        if hasPrefixFold(s, "author:") || hasPrefixFold(s, "version:") { idx = i + 1; continue }
        // Stop at first non-metadata, non-empty line
        break
    }
    return idx
}

func countPrefix(s string, r byte) int {
    n := 0
    for i := 0; i < len(s) && s[i] == r; i++ { n++ }
    return n
}

func hasPrefixFold(s, prefix string) bool {
    if len(s) < len(prefix) { return false }
    return strings.EqualFold(s[:len(prefix)], prefix)
}

func containsHeadingCase(markdown, title string) bool {
    t := strings.ToLower(strings.TrimSpace(title))
    for _, line := range strings.Split(markdown, "\n") {
        s := strings.TrimSpace(line)
        if !strings.HasPrefix(s, "#") { continue }
        // strip leading # and spaces
        for len(s) > 0 && s[0] == '#' { s = s[1:] }
        s = strings.TrimSpace(s)
        if strings.EqualFold(s, t) { return true }
    }
    return false
}

// makeSlugForToC mirrors the slug rules used by validate.distribution.makeSlug
// to ensure in-document anchor links resolve to headings in the same document.
func makeSlugForToC(s string) string {
    s = strings.ToLower(strings.TrimSpace(s))
    var b strings.Builder
    lastHyphen := false
    for _, r := range s {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
            b.WriteRune(r)
            lastHyphen = false
            continue
        }
        if r == ' ' || r == '-' || r == '_' {
            if !lastHyphen {
                b.WriteByte('-')
                lastHyphen = true
            }
            continue
        }
        // drop other characters
    }
    out := b.String()
    out = strings.Trim(out, "-")
    return out
}
