package validate

import (
    "fmt"
    "regexp"
    "strings"
)

// ValidateDistributionReady verifies minimal publication metadata and internal anchors.
// It requires the document to contain:
// - First non-empty line: H1 title
// - Second non-empty line: ISO date (YYYY-MM-DD)
// - An "Author: ..." line near the top
// - A "Version: ..." line near the top (semver-ish)
// - In-document links of the form [..](#anchor) must target an existing heading
// If expectedAuthor or expectedVersion are non-empty, they must match exactly (case-insensitive).
func ValidateDistributionReady(markdown string, expectedAuthor string, expectedVersion string) error {
    if err := validateMetadata(markdown, expectedAuthor, expectedVersion); err != nil {
        return err
    }
    if err := validateAnchorLinks(markdown); err != nil {
        return err
    }
    return nil
}

// validateMetadata checks for title/date and Author/Version metadata lines.
func validateMetadata(markdown string, expectedAuthor string, expectedVersion string) error {
    lines := splitLines(markdown)
    // Locate first two non-empty lines
    firstIdx, secondIdx := -1, -1
    for i := 0; i < len(lines); i++ {
        if trimSpace(lines[i]) == "" { continue }
        if firstIdx == -1 { firstIdx = i; continue }
        if secondIdx == -1 { secondIdx = i; break }
    }
    if firstIdx == -1 || secondIdx == -1 {
        return fmt.Errorf("missing title or date line in header")
    }
    if !isHeading(trimSpace(lines[firstIdx])) {
        return fmt.Errorf("first non-empty line must be an H1 title")
    }
    if !dateRe.MatchString(trimSpace(lines[secondIdx])) {
        return fmt.Errorf("second non-empty line must be an ISO date (YYYY-MM-DD)")
    }

    // Scan first N lines for metadata
    authorLineRe := regexp.MustCompile(`(?i)^\s*author\s*:\s*(.+\S)\s*$`)
    versionLineRe := regexp.MustCompile(`(?i)^\s*version\s*:\s*([vV]?\d+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)\s*$`)
    foundAuthor := ""
    foundVersion := ""
    limit := len(lines)
    if limit > 40 { limit = 40 }
    for i := 0; i < limit; i++ {
        s := trimSpace(lines[i])
        if s == "" { continue }
        if m := authorLineRe.FindStringSubmatch(s); m != nil {
            foundAuthor = strings.TrimSpace(m[1])
        }
        if m := versionLineRe.FindStringSubmatch(s); m != nil {
            foundVersion = strings.TrimSpace(m[1])
        }
        if foundAuthor != "" && foundVersion != "" { break }
    }
    if foundAuthor == "" {
        return fmt.Errorf("metadata missing: Author: <name>")
    }
    if foundVersion == "" {
        return fmt.Errorf("metadata missing: Version: <semver>")
    }
    if strings.TrimSpace(expectedAuthor) != "" && !equalsIgnoreCase(foundAuthor, strings.TrimSpace(expectedAuthor)) {
        return fmt.Errorf("author mismatch: got %q want %q", foundAuthor, expectedAuthor)
    }
    if strings.TrimSpace(expectedVersion) != "" && !equalsIgnoreCase(foundVersion, strings.TrimSpace(expectedVersion)) {
        return fmt.Errorf("version mismatch: got %q want %q", foundVersion, expectedVersion)
    }
    return nil
}

// validateAnchorLinks ensures that in-document anchor links reference existing headings.
func validateAnchorLinks(markdown string) error {
    lines := splitLines(markdown)
    slugs := map[string]struct{}{}
    for _, line := range lines {
        s := trimSpace(line)
        if isHeading(s) {
            txt := stripHeading(s)
            slug := makeSlug(txt)
            if slug != "" {
                slugs[slug] = struct{}{}
            }
        }
    }
    linkRe := regexp.MustCompile(`\[[^\]]+\]\((#[^)]+)\)`) // [text](#anchor)
    missing := []string{}
    for _, line := range lines {
        for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
            if len(m) < 2 { continue }
            target := strings.TrimSpace(m[1])
            target = strings.TrimPrefix(target, "#")
            slug := makeSlug(target)
            if _, ok := slugs[slug]; !ok {
                missing = append(missing, slug)
            }
        }
    }
    if len(missing) > 0 {
        uniq := map[string]struct{}{}
        out := make([]string, 0, len(missing))
        for _, s := range missing {
            if s == "" { continue }
            if _, ok := uniq[s]; ok { continue }
            uniq[s] = struct{}{}
            out = append(out, s)
        }
        if len(out) > 0 {
            return fmt.Errorf("broken anchor links to missing headings: %v", out)
        }
    }
    return nil
}

func makeSlug(s string) string {
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
