package validate

import (
    "fmt"
    "net/url"
    "regexp"
    "sort"
    "strings"
    "time"
)

// Citations represents the validation result for inline [n] citations
// relative to a references list of length N.
type Citations struct {
    // InRange lists citation indices that are valid (1..N)
    InRange []int
    // OutOfRange lists citation indices that reference >N or <1
    OutOfRange []int
    // MissingReferences is true if N == 0 while citations exist
    MissingReferences bool
}

var citeRe = regexp.MustCompile(`\[(\d+)\]`)

// ValidateCitations scans the markdown body for [n] patterns and compares
// against the number of references.
func ValidateCitations(markdown string, numReferences int) Citations {
    matches := citeRe.FindAllStringSubmatch(markdown, -1)
    seen := map[int]struct{}{}
    var inRange []int
    var outRange []int
    for _, m := range matches {
        if len(m) != 2 {
            continue
        }
        var n int
        for _, ch := range m[1] {
            n = n*10 + int(ch-'0')
        }
        if _, ok := seen[n]; ok {
            continue
        }
        seen[n] = struct{}{}
        if n >= 1 && n <= numReferences {
            inRange = append(inRange, n)
        } else {
            outRange = append(outRange, n)
        }
    }
    sort.Ints(inRange)
    sort.Ints(outRange)
    return Citations{InRange: inRange, OutOfRange: outRange, MissingReferences: numReferences == 0 && len(matches) > 0}
}

// EnsureReferencesSection tries to count reference entries by scanning for a
// heading that looks like 'References' and counting numbered list items that
// follow until a blank line or new heading. This is a minimal, deterministic
// method suitable for baseline validation without heavy markdown parsing.
func EnsureReferencesSection(markdown string) (num int, ok bool) {
    // Very small state machine over lines
    lines := splitLines(markdown)
    inRefs := false
    for i := 0; i < len(lines); i++ {
        line := trimSpace(lines[i])
        if line == "" {
            if inRefs && num > 0 {
                return num, true
            }
            continue
        }
        if isHeading(line) {
            if inRefs {
                // Next heading ends references block
                return num, num > 0
            }
            if equalsIgnoreCase(stripHeading(line), "references") {
                inRefs = true
                continue
            }
        }
        if inRefs {
            // Count markdown numbered list items: "1. ..."
            if isNumberedItem(line) {
                num++
                continue
            }
        }
    }
    if inRefs && num > 0 {
        return num, true
    }
    return 0, false
}

// ValidateReferencesCompleteness checks that each numbered reference item
// contains both a human-readable title and at least one full URL.
// Title detection is heuristic: after stripping the leading numeric marker and
// any URLs, the remaining text must contain at least a few letters or a
// Markdown link label like [Title].
func ValidateReferencesCompleteness(markdown string) (incompleteIndices []int) {
    lines := splitLines(markdown)
    inRefs := false
    itemOrder := 0
    for i := 0; i < len(lines); i++ {
        line := trimSpace(lines[i])
        if line == "" {
            if inRefs && itemOrder > 0 {
                // Allow early termination on blank line after some items
                return incompleteIndices
            }
            continue
        }
        if isHeading(line) {
            if inRefs {
                // End of references section
                return incompleteIndices
            }
            if equalsIgnoreCase(stripHeading(line), "references") {
                inRefs = true
                continue
            }
        }
        if inRefs {
            if isNumberedItem(line) {
                itemOrder++
                content := stripNumberedPrefix(line)
                hasURL := containsURL(content)
                hasTitle := containsTitleText(content)
                if !hasURL || !hasTitle {
                    incompleteIndices = append(incompleteIndices, itemOrder)
                }
            }
        }
    }
    return incompleteIndices
}

var urlRe = regexp.MustCompile(`https?://[^\s)]+`)
var linkLabelRe = regexp.MustCompile(`\[[^\]]+\]\(`) // e.g., [Title](
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func containsURL(s string) bool {
    return urlRe.FindStringIndex(s) != nil
}

func containsTitleText(s string) bool {
    // If there is an explicit markdown link label, that's a title.
    if linkLabelRe.FindStringIndex(s) != nil {
        return true
    }
    // Remove URLs and common separators, then check for letters
    withoutURLs := urlRe.ReplaceAllString(s, "")
    // Drop common separators like dash, em dash, colon, parentheses
    cleaned := make([]rune, 0, len(withoutURLs))
    for _, r := range withoutURLs {
        switch r {
        case '—', '-', '–', ':', '(', ')', '[', ']', ' ', '\t':
            continue
        default:
            cleaned = append(cleaned, r)
        }
    }
    // Count ASCII letters as a simple proxy for a human-readable title
    letters := 0
    for _, r := range cleaned {
        if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
            letters++
            if letters >= 3 {
                return true
            }
        }
    }
    return false
}

func stripNumberedPrefix(s string) string {
    // Assumes isNumberedItem(s) was true
    i := 0
    for i < len(s) && s[i] >= '0' && s[i] <= '9' {
        i++
    }
    if i < len(s) && s[i] == '.' {
        i++
    }
    if i < len(s) && s[i] == ' ' {
        i++
    }
    return trimSpace(s[i:])
}

func splitLines(s string) []string {
    var out []string
    start := 0
    for i := 0; i < len(s); i++ {
        if s[i] == '\n' {
            out = append(out, s[start:i])
            start = i + 1
        }
    }
    out = append(out, s[start:])
    return out
}

func trimSpace(s string) string {
    i := 0
    j := len(s)
    for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') {
        i++
    }
    for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
        j--
    }
    return s[i:j]
}

func isHeading(s string) bool {
    // e.g., "# References" .. "###### References"
    if len(s) == 0 {
        return false
    }
    i := 0
    for i < len(s) && s[i] == '#' {
        i++
    }
    return i > 0 && i <= 6 && (i < len(s) && s[i] == ' ')
}

func stripHeading(s string) string {
    i := 0
    for i < len(s) && s[i] == '#' {
        i++
    }
    if i < len(s) && s[i] == ' ' {
        i++
    }
    return trimSpace(s[i:])
}

func equalsIgnoreCase(a, b string) bool {
    if len(a) != len(b) {
        return false
    }
    for i := 0; i < len(a); i++ {
        ca := a[i]
        cb := b[i]
        if 'A' <= ca && ca <= 'Z' {
            ca = ca - 'A' + 'a'
        }
        if 'A' <= cb && cb <= 'Z' {
            cb = cb - 'A' + 'a'
        }
        if ca != cb {
            return false
        }
    }
    return true
}

func isNumberedItem(s string) bool {
    // Match simple "N. text" at start
    i := 0
    for i < len(s) && s[i] >= '0' && s[i] <= '9' {
        i++
    }
    if i == 0 {
        return false
    }
    if i >= len(s) || s[i] != '.' {
        return false
    }
    if i+1 < len(s) && s[i+1] == ' ' {
        return true
    }
    return false
}

// ValidateReport performs basic post-generation checks and returns an error if
// the document violates the citation contract.
func ValidateReport(markdown string) error {
    n, ok := EnsureReferencesSection(markdown)
    if !ok {
        return fmt.Errorf("references section missing or empty")
    }
    if bad := ValidateReferencesCompleteness(markdown); len(bad) > 0 {
        return fmt.Errorf("incomplete references (need title and full URL) at items: %v", bad)
    }
    c := ValidateCitations(markdown, n)
    if c.MissingReferences {
        return fmt.Errorf("citations present but no references")
    }
    if len(c.OutOfRange) > 0 {
        return fmt.Errorf("out-of-range citations: %v", c.OutOfRange)
    }
    return nil
}

// ReferenceQualityPolicy configures quality and mix checks over the references
// list. It is intentionally simple and deterministic, relying only on the
// markdown text without additional network calls.
type ReferenceQualityPolicy struct {
    // RequireAtLeastOnePreferred enforces that at least one reference comes
    // from a preferred host (peer-reviewed venue or standards body).
    RequireAtLeastOnePreferred bool
    // MinPreferredFraction, when >0, enforces that at least this fraction of
    // references are from preferred hosts. Example: 0.3 means ≥30%.
    MinPreferredFraction float64
    // PreferredHostPatterns lists host patterns considered preferred. A pattern
    // matches when host equals the pattern or ends with "."+pattern.
    PreferredHostPatterns []string

    // MaxPerDomainFraction, when >0, rejects over-reliance on a single host if
    // any single domain exceeds this fraction of total references.
    MaxPerDomainFraction float64
    // MaxPerDomain, when >0, caps the absolute number of references per domain.
    MaxPerDomain int

    // RecentWithinYears, when >0, defines the cutoff for a reference to count
    // as "recent" based on a four-digit year found in the reference line.
    // Example: 5 means year >= (Now().Year()-5).
    RecentWithinYears int
    // MinRecentFraction, when >0 and RecentWithinYears>0, enforces that at
    // least this fraction of references are recent.
    MinRecentFraction float64
    // RecencyExemptHostPatterns lists host patterns that are exempt from
    // recency checks (e.g., standards like RFCs which remain valid for years).
    RecencyExemptHostPatterns []string

    // Now allows tests to inject a fixed time. If nil, time.Now is used.
    Now func() time.Time
}

// ValidateReferenceQuality parses the references section in the provided
// markdown and enforces the configured ReferenceQualityPolicy. It returns an
// error describing the first violated constraint, or nil when the policy is
// satisfied or not applicable.
func ValidateReferenceQuality(markdown string, policy ReferenceQualityPolicy) error {
    refs := extractReferences(markdown)
    if len(refs) == 0 {
        // Nothing to check; leave to other validators to require references.
        return nil
    }

    // Build helper predicates
    isPreferred := func(host string) bool {
        h := strings.ToLower(host)
        for _, p := range policy.PreferredHostPatterns {
            pp := strings.ToLower(strings.TrimSpace(p))
            if pp == "" {
                continue
            }
            if h == pp || strings.HasSuffix(h, "."+pp) {
                return true
            }
        }
        return false
    }
    isRecencyExempt := func(host string) bool {
        h := strings.ToLower(host)
        for _, p := range policy.RecencyExemptHostPatterns {
            pp := strings.ToLower(strings.TrimSpace(p))
            if pp == "" {
                continue
            }
            if h == pp || strings.HasSuffix(h, "."+pp) {
                return true
            }
        }
        return false
    }

    // Preferred sources checks
    if policy.RequireAtLeastOnePreferred || policy.MinPreferredFraction > 0 {
        preferred := 0
        for _, r := range refs {
            if isPreferred(r.Host) {
                preferred++
            }
        }
        if policy.RequireAtLeastOnePreferred && preferred == 0 {
            return fmt.Errorf("reference quality: expected at least one preferred source (peer-reviewed or standards)")
        }
        if policy.MinPreferredFraction > 0 {
            frac := float64(preferred) / float64(len(refs))
            if frac+1e-9 < policy.MinPreferredFraction { // tiny epsilon for float comparisons
                return fmt.Errorf("reference quality: preferred source fraction %.2f < required %.2f", frac, policy.MinPreferredFraction)
            }
        }
    }

    // Over-reliance checks
    if policy.MaxPerDomain > 0 || policy.MaxPerDomainFraction > 0 {
        counts := map[string]int{}
        for _, r := range refs {
            counts[r.Host]++
        }
        for host, n := range counts {
            if policy.MaxPerDomain > 0 && n > policy.MaxPerDomain {
                return fmt.Errorf("reference mix: too many references from %s (%d > %d)", host, n, policy.MaxPerDomain)
            }
            if policy.MaxPerDomainFraction > 0 {
                frac := float64(n) / float64(len(refs))
                if frac > policy.MaxPerDomainFraction+1e-9 {
                    return fmt.Errorf("reference mix: domain %s dominates with fraction %.2f > %.2f", host, frac, policy.MaxPerDomainFraction)
                }
            }
        }
    }

    // Recency checks
    if policy.RecentWithinYears > 0 && policy.MinRecentFraction > 0 {
        now := time.Now
        if policy.Now != nil {
            now = policy.Now
        }
        cutoff := now().Year() - policy.RecentWithinYears
        recent := 0
        total := 0
        for _, r := range refs {
            if isRecencyExempt(r.Host) {
                // Treat exempt hosts as recent-neutral; neither help nor hurt.
                continue
            }
            total++
            if r.Year >= cutoff && r.Year <= now().Year() && r.Year != 0 {
                recent++
            }
        }
        if total > 0 { // if all were exempt, recency check is vacuously satisfied
            frac := float64(recent) / float64(total)
            if frac+1e-9 < policy.MinRecentFraction {
                return fmt.Errorf("reference recency: fraction of recent items %.2f < required %.2f (cutoff year %d)", frac, policy.MinRecentFraction, cutoff)
            }
        }
    }
    return nil
}

type referenceEntry struct {
    Index int
    Title string
    URL   string
    Host  string
    Year  int
}

// extractReferences scans the markdown References section and extracts the URL,
// host, and any four-digit year present on the same line as a heuristic
// publication/last-updated date.
func extractReferences(markdown string) []referenceEntry {
    lines := splitLines(markdown)
    inRefs := false
    order := 0
    var out []referenceEntry
    for i := 0; i < len(lines); i++ {
        line := trimSpace(lines[i])
        if line == "" {
            if inRefs && order > 0 {
                break
            }
            continue
        }
        if isHeading(line) {
            if inRefs {
                break
            }
            if equalsIgnoreCase(stripHeading(line), "references") {
                inRefs = true
                continue
            }
        }
        if !inRefs {
            continue
        }
        if isNumberedItem(line) {
            order++
            content := stripNumberedPrefix(line)
            u := firstURL(content)
            host := ""
            if u != "" {
                if pu, err := url.Parse(strings.TrimSpace(u)); err == nil {
                    host = strings.ToLower(pu.Host)
                }
            }
            yr := detectYearInText(content)
            title := strings.TrimSpace(strings.ReplaceAll(content, u, ""))
            out = append(out, referenceEntry{Index: order, Title: title, URL: u, Host: host, Year: yr})
        }
    }
    return out
}

func firstURL(s string) string {
    loc := urlRe.FindStringIndex(s)
    if loc == nil {
        return ""
    }
    return s[loc[0]:loc[1]]
}

var yearRe = regexp.MustCompile(`(?:\(|\b)(19\d{2}|20\d{2}|21\d{2})(?:\)|\b)`) // 1900..2199 with simple bounds

func detectYearInText(s string) int {
    // Favor the last year on the line, which often reflects the most recent.
    matches := yearRe.FindAllStringSubmatch(s, -1)
    if len(matches) == 0 {
        return 0
    }
    last := matches[len(matches)-1][1]
    // Fast atoi for 4-digit year
    y := 0
    for _, ch := range last {
        y = y*10 + int(ch-'0')
    }
    // clamp to a sane range just in case
    if y < 1900 || y > 2199 {
        return 0
    }
    return y
}

// ValidateStructure enforces a minimal Markdown output contract:
// - First non-empty line is a single H1 title ("# Title")
// - Second non-empty line is an ISO date (YYYY-MM-DD)
// - Body contains section headings that match the provided outline in order
//   (case-insensitive). Missing or out-of-order sections are reported.
// - Contains a "Risks and limitations" section (case-insensitive)
// - Contains a "References" section (already required by ValidateReport but
//   checked here for structure compliance as well)
// The function is deterministic and tolerant of blank lines.
func ValidateStructure(markdown string, outline []string) error {
    lines := splitLines(markdown)
    // Locate first two non-empty lines
    firstIdx, secondIdx := -1, -1
    for i := 0; i < len(lines); i++ {
        if trimSpace(lines[i]) == "" {
            continue
        }
        if firstIdx == -1 {
            firstIdx = i
            continue
        }
        if secondIdx == -1 {
            secondIdx = i
            break
        }
    }
    if firstIdx == -1 {
        return fmt.Errorf("document is empty; missing title")
    }
    first := trimSpace(lines[firstIdx])
    if !isHeading(first) || !(len(first) > 1 && first[0] == '#' && first[1] == ' ') {
        return fmt.Errorf("first non-empty line must be an H1 markdown heading")
    }
    // Ensure there is exactly one leading '#'
    hashes := 0
    for hashes < len(first) && first[hashes] == '#' {
        hashes++
    }
    if hashes != 1 {
        return fmt.Errorf("title must be a single '# ' H1 heading")
    }
    if secondIdx == -1 {
        return fmt.Errorf("second non-empty line must be an ISO date (YYYY-MM-DD)")
    }
    second := trimSpace(lines[secondIdx])
    if !dateRe.MatchString(second) {
        return fmt.Errorf("date line must be YYYY-MM-DD below title")
    }

    // Collect headings after the date line in order with their text and level
    type hd struct{ level int; text string }
    var heads []hd
    for i := secondIdx + 1; i < len(lines); i++ {
        line := trimSpace(lines[i])
        if !isHeading(line) {
            continue
        }
        // Count leading '#'
        lvl := 0
        for lvl < len(line) && line[lvl] == '#' {
            lvl++
        }
        txt := stripHeading(line)
        heads = append(heads, hd{level: lvl, text: txt})
    }

    // Verify presence and order of outline sections (case-insensitive match)
    if len(outline) > 0 {
        pos := 0
        for idx, want := range outline {
            found := false
            wanted := trimSpace(want)
            for ; pos < len(heads); pos++ {
                if equalsIgnoreCase(trimSpace(heads[pos].text), wanted) {
                    found = true
                    pos++
                    break
                }
            }
            if !found {
                return fmt.Errorf("missing or out-of-order outline section: %q (index %d)", want, idx)
            }
        }
    }

    // Ensure Risks and limitations section exists
    hasRisks := false
    hasRefs := false
    for _, h := range heads {
        if equalsIgnoreCase(trimSpace(h.text), "risks and limitations") {
            hasRisks = true
        }
        if equalsIgnoreCase(trimSpace(h.text), "references") {
            hasRefs = true
        }
    }
    if !hasRisks {
        return fmt.Errorf("missing 'Risks and limitations' section")
    }
    if !hasRefs {
        return fmt.Errorf("missing 'References' section heading")
    }

    // Enforce a sensible hierarchy: only one H1 overall
    h1count := 0
    for _, h := range heads {
        if h.level == 1 {
            h1count++
        }
    }
    if h1count > 0 {
        return fmt.Errorf("document must not contain additional H1 headings beyond the title")
    }
    return nil
}


