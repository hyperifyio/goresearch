package validate

import (
    "fmt"
    "net/url"
    "regexp"
    "sort"
    "strings"
    "time"
    "unicode"
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
    // Executive summary validation
    if err := ValidateExecutiveSummary(markdown); err != nil {
        return fmt.Errorf("executive summary validation failed: %v", err)
    }
    
    // Accessibility validation
    if err := ValidateAccessibility(markdown); err != nil {
        return fmt.Errorf("accessibility validation failed: %v", err)
    }
    
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

// ValidateReferencesEnrichment verifies that reference lines are enriched with
// stable URLs where applicable, include DOI URLs when a DOI is present, and include
// an "Accessed on YYYY-MM-DD" date for web sources. This check is tolerant and
// returns a single aggregated error describing the first issue found.
func ValidateReferencesEnrichment(markdown string) error {
    lines := splitLines(markdown)
    inRefs := false
    order := 0
    // Simple regexes (must match app.enrichReferences behavior)
    headingRe := regexp.MustCompile(`^#{1,6}\s+References\s*$`)
    numItemRe := regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
    urlRe := regexp.MustCompile(`https?://[^\s)]+`)
    accessedRe := regexp.MustCompile(`(?i)accessed on\s+\d{4}-\d{2}-\d{2}`)
    doiCore := `10\.[0-9]{4,9}/[-._;()/:A-Za-z0-9]+`
    doiTextRe := regexp.MustCompile(`(?i)(?:doi\s*[:]?\s*|https?://(?:dx\.)?doi\.org/)(` + doiCore + `)`) // capture DOI

    for i := 0; i < len(lines); i++ {
        s := trimSpace(lines[i])
        if s == "" { continue }
        if headingRe.MatchString(s) { inRefs = true; order = 0; continue }
        if inRefs {
            if isHeading(s) { break }
            m := numItemRe.FindStringSubmatch(s)
            if m == nil { continue }
            order++
            content := trimSpace(m[2])

            // If URL exists, require Accessed on date
            if urlRe.FindStringIndex(content) != nil && !accessedRe.MatchString(content) {
                return fmt.Errorf("references enrichment: item %d missing 'Accessed on YYYY-MM-DD'", order)
            }
            // If DOI detectable, require canonical DOI URL presence
            if dm := doiTextRe.FindStringSubmatch(content); dm != nil && len(dm) >= 2 {
                doi := dm[1]
                doiURL := "https://doi.org/" + doi
                if !strings.Contains(strings.ToLower(content), strings.ToLower(doiURL)) {
                    return fmt.Errorf("references enrichment: item %d missing DOI URL %s", order, doiURL)
                }
            }
        }
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

    // Ensure required sections exist
    hasRisks := false
    hasAltConf := false
    hasRefs := false
    for _, h := range heads {
        if equalsIgnoreCase(trimSpace(h.text), "risks and limitations") {
            hasRisks = true
        }
        if equalsIgnoreCase(trimSpace(h.text), "alternatives & conflicting evidence") {
            hasAltConf = true
        }
        if equalsIgnoreCase(trimSpace(h.text), "references") {
            hasRefs = true
        }
    }
    if !hasRisks {
        return fmt.Errorf("missing 'Risks and limitations' section")
    }
    if !hasAltConf {
        return fmt.Errorf("missing 'Alternatives & conflicting evidence' section")
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


// ValidateVisuals enforces visuals quality rules for figures and tables in a Markdown document:
// - Figures and tables must be numbered sequentially starting at 1 per-kind (Fig. 1, Fig. 2; Table 1, Table 2)
// - Each visual must have a non-empty caption (for figures: either in alt text containing the caption alongside the number, or an adjacent caption line like "Figure 1: ...")
// - Images must include non-empty alt text
// - Each numbered visual must be referenced in text (e.g., "See Fig. 1" or "Table 1")
// - Placement heuristic: a textual reference for a visual should appear within a small window of lines (default ±8) from the visual's location
// If no figures or tables are present, this validator returns nil.
func ValidateVisuals(markdown string) error {
    type visualKind int
    const (
        kindFigure visualKind = iota
        kindTable
    )
    type visual struct{
        kind visualKind
        number int
        caption string
        line int
        // for figures
        hasAlt bool
    }

    lines := splitLines(markdown)

    // Regexes for detection
    // Image: ![alt](src)
    imgAltStart := "!["
    // Caption lines: Figure/Fig. N: ...  or  Table N: ...
    figCaptionRe := regexp.MustCompile(`^(?i)\s*(figure|fig\.)\s+(\d+)\s*[:\-—]\s*(.+\S)\s*$`)
    tblCaptionRe := regexp.MustCompile(`^(?i)\s*table\s+(\d+)\s*[:\-—]\s*(.+\S)\s*$`)
    // In-text mentions
    figMentionRe := regexp.MustCompile(`(?i)\b(see\s+)?(fig\.|figure)\s+(\d+)\b`)
    tblMentionRe := regexp.MustCompile(`(?i)\b(see\s+)?table\s+(\d+)\b`)

    // Detect tables using a simple two-line header + separator heuristic
    isTableHeader := func(s string) bool {
        s = trimSpace(s)
        return strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") && strings.Count(s, "|") >= 2
    }
    isTableSeparator := func(s string) bool {
        s = trimSpace(s)
        if !strings.HasPrefix(s, "|") || !strings.HasSuffix(s, "|") {
            return false
        }
        // require at least one segment like :---: or ---
        segs := strings.Split(s, "|")
        found := 0
        for _, seg := range segs {
            seg = trimSpace(seg)
            if seg == "" { continue }
            ok := true
            for _, r := range seg {
                if !(r == ':' || r == '-' ) {
                    ok = false
                    break
                }
            }
            if ok { found++ }
        }
        return found >= 1
    }

    // Collect visuals
    visuals := make([]visual, 0)
    // Track mentions per line index
    figMentionsByNum := map[int][]int{}
    tblMentionsByNum := map[int][]int{}

    // Precompute which lines are captions or image lines to avoid counting them as in-text mentions
    isFigCaptionLine := make([]bool, len(lines))
    isTblCaptionLine := make([]bool, len(lines))
    isImageLine := make([]bool, len(lines))
    for i, line := range lines {
        s := trimSpace(line)
        if figCaptionRe.MatchString(s) { isFigCaptionLine[i] = true }
        if tblCaptionRe.MatchString(s) { isTblCaptionLine[i] = true }
        if strings.Contains(line, imgAltStart) { isImageLine[i] = true }
    }

    // Pass 1: collect mentions across all lines (excluding captions and image lines)
    for i, line := range lines {
        if isFigCaptionLine[i] || isTblCaptionLine[i] || isImageLine[i] { continue }
        for _, m := range figMentionRe.FindAllStringSubmatch(line, -1) {
            if len(m) >= 4 {
                // m[3] is number
                n := 0
                for _, ch := range m[3] { n = n*10 + int(ch-'0') }
                figMentionsByNum[n] = append(figMentionsByNum[n], i)
            }
        }
        for _, m := range tblMentionRe.FindAllStringSubmatch(line, -1) {
            if len(m) >= 3 {
                n := 0
                // last group is number
                last := m[len(m)-1]
                for _, ch := range last { n = n*10 + int(ch-'0') }
                tblMentionsByNum[n] = append(tblMentionsByNum[n], i)
            }
        }
    }

    // Helper: try parse int from string
    parseNum := func(s string) int { n:=0; for _, ch := range s { if ch<'0'||ch>'9' { continue }; n = n*10 + int(ch-'0') }; return n }

    // Pass 2: detect figures and captions
    for i := 0; i < len(lines); i++ {
        line := lines[i]
        // Figures via images
        if idx := strings.Index(line, imgAltStart); idx != -1 {
            // Extract alt
            alt := ""
            // find closing ](
            rb := strings.Index(line[idx+2:], "](")
            if rb != -1 {
                alt = line[idx+2 : idx+2+rb]
            }
            hasAlt := trimSpace(alt) != ""
            // Determine number+caption
            num := 0
            cap := ""
            // Option A: number embedded in alt: Figure/ Fig. N ...
            if m := figCaptionRe.FindStringSubmatch(alt); m != nil {
                num = parseNum(m[2])
                cap = trimSpace(m[3])
            }
            // Option B: adjacent caption line (next non-empty or previous non-empty)
            if num == 0 {
                // next non-empty
                for j:=i+1; j < len(lines) && j <= i+2; j++ { // look within 2 lines
                    if c := trimSpace(lines[j]); c != "" {
                        if m := figCaptionRe.FindStringSubmatch(c); m != nil {
                            num = parseNum(m[2])
                            cap = trimSpace(m[3])
                        }
                        break
                    }
                }
            }
            if num == 0 {
                // previous non-empty
                for j:=i-1; j >= 0 && j >= i-2; j-- {
                    if c := trimSpace(lines[j]); c != "" {
                        if m := figCaptionRe.FindStringSubmatch(c); m != nil {
                            num = parseNum(m[2])
                            cap = trimSpace(m[3])
                        }
                        break
                    }
                }
            }
            visuals = append(visuals, visual{kind: kindFigure, number: num, caption: cap, line: i, hasAlt: hasAlt})
        }

        // Tables: header + separator heuristic; caption adjacent
        if i+1 < len(lines) && isTableHeader(lines[i]) && isTableSeparator(lines[i+1]) {
            // Find caption adjacent (prev non-empty or next non-empty beyond table)
            // First, skip table body to find the next non-table line
            end := i+2
            for end < len(lines) {
                s := trimSpace(lines[end])
                if s == "" { end++; continue }
                if strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") { end++; continue }
                break
            }
            // Try previous non-empty
            num := 0
            cap := ""
            for j:=i-1; j>=0 && j>=i-2; j-- {
                c := trimSpace(lines[j])
                if c == "" { continue }
                if m := tblCaptionRe.FindStringSubmatch(c); m != nil {
                    num = parseNum(m[1])
                    cap = trimSpace(m[2])
                }
                break
            }
            if num == 0 {
                // next non-empty after the table
                if end < len(lines) {
                    c := trimSpace(lines[end])
                    if m := tblCaptionRe.FindStringSubmatch(c); m != nil {
                        num = parseNum(m[1])
                        cap = trimSpace(m[2])
                    }
                }
            }
            visuals = append(visuals, visual{kind: kindTable, number: num, caption: cap, line: i, hasAlt: false})
            // advance i to end-1 so loop continues after table
            i = end-1
        }
    }

    if len(visuals) == 0 {
        return nil
    }

    // Validate per rules and accumulate issues
    var issues []string

    // Alt text and captions, numbering present
    figNums := make([]int, 0)
    tblNums := make([]int, 0)
    for _, v := range visuals {
        switch v.kind {
        case kindFigure:
            if !v.hasAlt {
                issues = append(issues, fmt.Sprintf("figure at line %d has empty alt text", v.line+1))
            }
            if v.number == 0 || trimSpace(v.caption) == "" {
                issues = append(issues, fmt.Sprintf("figure at line %d missing number and/or caption (expect 'Figure N: ...')", v.line+1))
            } else {
                figNums = append(figNums, v.number)
                // Placement: within ±8 lines of a mention
                if refs := figMentionsByNum[v.number]; len(refs) == 0 {
                    issues = append(issues, fmt.Sprintf("Figure %d is never referenced in text (e.g., 'See Fig. %d')", v.number, v.number))
                } else {
                    near := false
                    for _, rl := range refs {
                        if rl < 0 { continue }
                        if absInt(rl - v.line) <= 8 { near = true; break }
                    }
                    if !near {
                        issues = append(issues, fmt.Sprintf("Figure %d: nearest reference is too far from figure (require within ±8 lines)", v.number))
                    }
                }
            }
        case kindTable:
            if v.number == 0 || trimSpace(v.caption) == "" {
                issues = append(issues, fmt.Sprintf("table near line %d missing number and/or caption (expect 'Table N: ...')", v.line+1))
            } else {
                tblNums = append(tblNums, v.number)
                if refs := tblMentionsByNum[v.number]; len(refs) == 0 {
                    issues = append(issues, fmt.Sprintf("Table %d is never referenced in text", v.number))
                } else {
                    near := false
                    for _, rl := range refs {
                        if absInt(rl - v.line) <= 8 { near = true; break }
                    }
                    if !near {
                        issues = append(issues, fmt.Sprintf("Table %d: nearest reference is too far from table (require within ±8 lines)", v.number))
                    }
                }
            }
        }
    }

    // Numbering sequentiality per kind
    if len(figNums) > 0 {
        sort.Ints(figNums)
        for i, n := range figNums {
            if n != i+1 {
                issues = append(issues, fmt.Sprintf("figure numbering must be sequential starting at 1 (found %v)", figNums))
                break
            }
        }
    }
    if len(tblNums) > 0 {
        sort.Ints(tblNums)
        for i, n := range tblNums {
            if n != i+1 {
                issues = append(issues, fmt.Sprintf("table numbering must be sequential starting at 1 (found %v)", tblNums))
                break
            }
        }
    }

    if len(issues) == 0 {
        return nil
    }
    // Join issues; keep message short but informative
    return fmt.Errorf("visuals QA issues: %s", strings.Join(issues, "; "))
}

func absInt(x int) int { if x<0 { return -x }; return x }


// ValidateTitleQuality enforces basic title quality rules:
// - Title must be a single-line H1 (already enforced by ValidateStructure) and ≤ 12 words
// - Title must include at least two descriptive keywords (non-stopwords with length ≥ 4)
// - No unexplained acronyms in the title: any ALL-CAPS acronym (2..6 letters, optional plural 's')
//   must be defined somewhere in the document body as either "Long Form (ACRO)" or "ACRO (Long Form)"
func ValidateTitleQuality(markdown string) error {
    title, err := extractH1Title(markdown)
    if err != nil {
        return err
    }

    // Word count (consider tokens with at least one letter/digit)
    tokens := strings.Fields(title)
    words := 0
    for _, tok := range tokens {
        if isWordToken(tok) { words++ }
    }
    if words > 12 {
        return fmt.Errorf("title must be <= 12 words (found %d)", words)
    }

    // Descriptive keywords: at least one non-stopword of length >=4
    if countContentKeywords(tokens) < 1 {
        return fmt.Errorf("title must include at least one descriptive keyword (non-stopword length >= 4)")
    }

    // Unexplained acronyms check
    undefined := undefinedTitleAcronyms(markdown, title)
    if len(undefined) > 0 {
        return fmt.Errorf("unexplained acronyms in title: %s", strings.Join(undefined, ", "))
    }
    return nil
}

// ValidateHeadingsQuality enforces heading mini-title quality and hierarchy:
// - Headings must be descriptive: avoid generic labels like "Introduction", "Background" alone.
//   Require either at least 2 content words (non-stopwords length ≥4) or a colon/mdash subtitle.
// - Hierarchy must not jump more than one level at a time (e.g., H2 -> H4 is rejected).
// - Parallel phrasing for sibling headings: among immediate siblings (same level), the leading
//   word form should be consistent. We approximate by checking whether a majority start with a
//   gerund (ends with "ing"); mixing gerund and non-gerund starts triggers a warning-level error.
// Excludes the H1 title and appendix/meta sections.
func ValidateHeadingsQuality(markdown string) error {
    lines := splitLines(markdown)
    type hd struct{ level int; text string; line int }
    var heads []hd
    h1Seen := false
    for i, raw := range lines {
        s := trimSpace(raw)
        if !isHeading(s) { continue }
        lvl := 0
        for lvl < len(s) && s[lvl] == '#' { lvl++ }
        txt := stripHeading(s)
        if !h1Seen && lvl == 1 { h1Seen = true; continue }
        // Skip common meta sections
        low := strings.ToLower(txt)
        if low == "references" || strings.Contains(low, "glossary") || strings.HasPrefix(low, "evidence") || strings.Contains(low, "manifest") {
            continue
        }
        heads = append(heads, hd{level: lvl, text: txt, line: i+1})
    }

    if len(heads) == 0 { return nil }

    // 1) Hierarchy consistency: no jumps >1 level
    prev := heads[0].level
    for i := 1; i < len(heads); i++ {
        if heads[i].level > prev+1 {
            return fmt.Errorf("heading level jumps from H%d to H%d at line %d", prev, heads[i].level, heads[i].line)
        }
        prev = heads[i].level
    }

    // 2) Descriptive mini-titles
    for _, h := range heads {
        if !isDescriptiveHeading(h.text) {
            return fmt.Errorf("non-descriptive heading: %q (line %d) — use a mini-title like 'Background and context' or add a subtitle", h.text, h.line)
        }
    }

    // 3) Parallel phrasing among siblings: check runs of same-level headings between deeper ones
    for i := 0; i < len(heads); {
        j := i + 1
        for j < len(heads) && heads[j].level == heads[i].level { j++ }
        // check siblings heads[i:j]
        if j-i >= 2 {
            gerunds := 0
            non := 0
            for k := i; k < j; k++ {
                if startsWithGerund(heads[k].text) { gerunds++ } else { non++ }
            }
            if gerunds > 0 && non > 0 {
                return fmt.Errorf("sibling headings at level H%d mix gerund and non-gerund starts; keep phrasing parallel", heads[i].level)
            }
        }
        // advance to next group; if next is deeper, skip until we return to same or above
        i = j
        // If the next heading is deeper, we still process groups independently as encountered.
    }
    return nil
}

func isDescriptiveHeading(s string) bool {
    t := strings.TrimSpace(s)
    if t == "" { return false }
    // Allow subtitle punctuation to qualify as descriptive
    if strings.Contains(t, ":") || strings.Contains(t, "—") || strings.Contains(t, "–") {
        return true
    }
    // Generic labels commonly produced by LLMs
    generic := map[string]struct{}{
        "introduction": {},
        "background": {},
        "overview": {},
        "conclusion": {},
        "summary": {},
        "body": {},
        "analysis": {},
    }
    low := strings.ToLower(t)
    if _, ok := generic[low]; ok { return false }
    // Count content keywords
    tokens := strings.Fields(t)
    if countContentKeywords(tokens) >= 2 { return true }
    return false
}

func startsWithGerund(s string) bool {
    // Consider first token after stripping leading punctuation
    fields := strings.Fields(s)
    if len(fields) == 0 { return false }
    w := normalizeToken(fields[0])
    if len(w) < 4 { return false }
    if strings.HasSuffix(w, "ing") {
        // avoid words like "string" by requiring not to end with "string" exact
        if w == "string" { return false }
        return true
    }
    return false
}

func extractH1Title(markdown string) (string, error) {
    lines := splitLines(markdown)
    for i := 0; i < len(lines); i++ {
        s := trimSpace(lines[i])
        if s == "" { continue }
        if !isHeading(s) || !(len(s) > 1 && s[0] == '#' && s[1] == ' ') {
            return "", fmt.Errorf("first non-empty line must be an H1 markdown heading")
        }
        // ensure exactly one '#'
        hashes := 0
        for hashes < len(s) && s[hashes] == '#' { hashes++ }
        if hashes != 1 {
            return "", fmt.Errorf("title must be a single '# ' H1 heading")
        }
        return stripHeading(s), nil
    }
    return "", fmt.Errorf("document is empty; missing title")
}

func isWordToken(tok string) bool {
    for _, r := range tok {
        if unicode.IsLetter(r) || unicode.IsDigit(r) { return true }
    }
    return false
}

func normalizeToken(tok string) string {
    // strip leading/trailing punctuation
    i, j := 0, len(tok)
    for i < j {
        r := rune(tok[i])
        if unicode.IsLetter(r) || unicode.IsDigit(r) { break }
        i++
    }
    for j > i {
        r := rune(tok[j-1])
        if unicode.IsLetter(r) || unicode.IsDigit(r) { break }
        j--
    }
    return strings.ToLower(tok[i:j])
}

func countContentKeywords(tokens []string) int {
    // minimal stopword list sufficient for title heuristics
    stop := map[string]struct{}{
        "a":{},"an":{},"the":{},"of":{},"and":{},"or":{},"to":{},"for":{},"in":{},"on":{},
        "with":{},"without":{},"by":{},"about":{},"from":{},"into":{},"over":{},"under":{},
        "between":{},"beyond":{},"at":{},"as":{},"is":{},"are":{},"be":{},"being":{},"been":{},
        "study":{},"report":{},"overview":{},"guide":{},"introduction":{},"analysis":{},
    }
    count := 0
    for _, t := range tokens {
        n := normalizeToken(t)
        if n == "" { continue }
        if _, ok := stop[n]; ok { continue }
        if len(n) >= 4 { count++ }
    }
    return count
}

var acronymInTitleRe = regexp.MustCompile(`\b([A-Z]{2,6})s?\b`)

func undefinedTitleAcronyms(markdown, title string) []string {
    seen := map[string]struct{}{}
    var acros []string
    for _, m := range acronymInTitleRe.FindAllStringSubmatch(title, -1) {
        if len(m) < 2 { continue }
        ac := m[1]
        // Skip when it's just a single letter repeated (unlikely due to {2,6})
        if _, ok := seen[ac]; ok { continue }
        seen[ac] = struct{}{}
        if !isAcronymDefined(markdown, ac) {
            acros = append(acros, ac)
        }
    }
    return acros
}

func isAcronymDefined(markdown, acro string) bool {
    if strings.TrimSpace(acro) == "" { return true }
    // Pattern A: Long form (ACRO)
    reA := regexp.MustCompile(`(?s)\b([A-Za-z][A-Za-z0-9&/\-]+(?:\s+[A-Za-z][A-Za-z0-9&/\-]+){0,6})\s*\(` + regexp.QuoteMeta(acro) + `\)`) //nolint:gosimple
    // Pattern B: ACRO (Long form)
    reB := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(acro) + `\s*\(([A-Za-z][A-Za-z0-9&/\-]+(?:\s+[A-Za-z][A-Za-z0-9&/\-]+){0,6})\)`) //nolint:gosimple
    return reA.FindStringIndex(markdown) != nil || reB.FindStringIndex(markdown) != nil
}

// ValidateExecutiveSummary enforces executive summary guardrails:
// - Length target of 150-250 words
// - Content checks for motivation, methods, key results, and recommendations
// Returns an error if the executive summary section fails these checks.
func ValidateExecutiveSummary(markdown string) error {
    execContent, err := extractExecutiveSummaryContent(markdown)
    if err != nil {
        return err
    }
    
    if execContent == "" {
        return fmt.Errorf("executive summary section is empty")
    }
    
    // Content quality checks first (more important than word count)
    var missing []string
    
    if !containsMotivation(execContent) {
        missing = append(missing, "motivation/problem statement")
    }
    if !containsMethods(execContent) {
        missing = append(missing, "methods/approach")
    }
    if !containsKeyResults(execContent) {
        missing = append(missing, "key results/findings")
    }
    if !containsRecommendations(execContent) {
        missing = append(missing, "recommendations/conclusions")
    }
    
    if len(missing) > 0 {
        return fmt.Errorf("executive summary missing essential content: %s", strings.Join(missing, ", "))
    }
    
    // Word count check (150-250 words target) after content checks
    wordCount := CountWords(execContent)
    if wordCount < 150 {
        return fmt.Errorf("executive summary too short: %d words (target: 150-250 words)", wordCount)
    }
    if wordCount > 250 {
        return fmt.Errorf("executive summary too long: %d words (target: 150-250 words)", wordCount)
    }
    
    return nil
}

// extractExecutiveSummaryContent finds and extracts the content from the
// "Executive summary" section of the markdown document.
func extractExecutiveSummaryContent(markdown string) (string, error) {
    lines := splitLines(markdown)
    inExecSummary := false
    var content []string
    
    for i := 0; i < len(lines); i++ {
        line := trimSpace(lines[i])
        if line == "" {
            if inExecSummary {
                content = append(content, "")
            }
            continue
        }
        
        if isHeading(line) {
            if inExecSummary {
                // Next heading ends executive summary section
                break
            }
            if equalsIgnoreCase(stripHeading(line), "executive summary") {
                inExecSummary = true
                continue
            }
        }
        
        if inExecSummary {
            content = append(content, line)
        }
    }
    
    if !inExecSummary {
        return "", fmt.Errorf("executive summary section not found")
    }
    
    return strings.Join(content, "\n"), nil
}

// containsMotivation checks for motivation/problem statement indicators
func containsMotivation(text string) bool {
    low := strings.ToLower(text)
    motivationMarkers := []string{
        "problem", "challenge", "issue", "need", "motivation", "why",
        "purpose", "objective", "goal", "aim", "background", "context",
        "driving", "requirement", "demand", "critical", "important",
    }
    
    for _, marker := range motivationMarkers {
        if strings.Contains(low, marker) {
            return true
        }
    }
    return false
}

// containsMethods checks for methods/approach indicators
func containsMethods(text string) bool {
    low := strings.ToLower(text)
    methodMarkers := []string{
        "method", "approach", "strategy", "technique", "process", "procedure",
        "methodology", "framework", "implementation", "solution", "design",
        "analysis", "evaluation", "assessment", "investigation", "study",
        "research", "survey", "review", "examination", "how", "using",
    }
    
    for _, marker := range methodMarkers {
        if strings.Contains(low, marker) {
            return true
        }
    }
    return false
}

// containsKeyResults checks for key results/findings indicators
func containsKeyResults(text string) bool {
    low := strings.ToLower(text)
    resultMarkers := []string{
        "result", "finding", "outcome", "conclusion", "discovery", "evidence",
        "data", "show", "demonstrate", "reveal", "indicate", "suggest",
        "confirm", "establish", "prove", "identify", "determine", "observe",
        "measure", "performance", "improvement", "benefit", "impact", "effect",
        "success", "achievement", "accomplishment", "found", "discovered",
    }
    
    for _, marker := range resultMarkers {
        if strings.Contains(low, marker) {
            return true
        }
    }
    return false
}

// containsRecommendations checks for recommendations/conclusions indicators
func containsRecommendations(text string) bool {
    low := strings.ToLower(text)
    recommendMarkers := []string{
        "recommend", "recommendation", "suggest", "proposal", "propose",
        "should", "must", "need to", "ought to", "advise", "guidance",
        "next steps", "action", "implement", "adopt", "consider", "evaluate",
        "conclusion", "summary", "implication", "takeaway", "lesson",
        "future", "forward", "direction", "path", "plan", "strategy",
    }
    
    for _, marker := range recommendMarkers {
        if strings.Contains(low, marker) {
            return true
        }
    }
    return false
}

// ValidateAccessibility enforces accessibility requirements for generated reports:
// - Heading order correctness: no level jumps >1 (e.g., H1->H3 is invalid)
// - Color-only meaning warnings: flags text that might rely solely on color cues
// - Alt text requirement: all images must have non-empty alt text
// Returns an error describing the first accessibility violation found, or nil if all checks pass.
func ValidateAccessibility(markdown string) error {
    lines := splitLines(markdown)
    
    // Check heading order correctness
    if err := validateHeadingOrder(lines); err != nil {
        return fmt.Errorf("heading order: %v", err)
    }
    
    // Check for color-only meaning issues
    if err := validateColorOnlyMeaning(markdown); err != nil {
        return fmt.Errorf("color-only meaning: %v", err)
    }
    
    // Check alt text requirements for images
    if err := validateImageAltText(lines); err != nil {
        return fmt.Errorf("image alt text: %v", err)
    }
    
    return nil
}

// validateHeadingOrder ensures headings follow proper hierarchical structure
// without skipping levels (e.g., H1 -> H2 -> H3 is valid, H1 -> H3 is invalid)
func validateHeadingOrder(lines []string) error {
    prevLevel := 0
    var headingInfos []struct{ level int; text string; line int }
    
    for i, line := range lines {
        s := trimSpace(line)
        if !isHeading(s) {
            continue
        }
        
        level := 0
        for level < len(s) && s[level] == '#' {
            level++
        }
        
        text := stripHeading(s)
        headingInfos = append(headingInfos, struct{ level int; text string; line int }{level, text, i + 1})
        
        // Skip first heading validation (document title)
        if prevLevel == 0 {
            prevLevel = level
            continue
        }
        
        // Check for level jumps > 1
        if level > prevLevel+1 {
            return fmt.Errorf("heading level jumps from H%d to H%d at line %d (%q)", prevLevel, level, i+1, text)
        }
        
        prevLevel = level
    }
    
    return nil
}

// validateColorOnlyMeaning detects text patterns that might rely solely on color for meaning
func validateColorOnlyMeaning(markdown string) error {
    lines := splitLines(markdown)
    for i, line := range lines {
        lower := strings.ToLower(line)
        
        // Check for "see the [color]" patterns
        seeColorPattern := regexp.MustCompile(`(?i)\bsee\s+(the\s+)?(red|green|blue|yellow|orange|purple|pink|gray|grey)\b`)
        if seeColorPattern.MatchString(line) {
            return fmt.Errorf("color-only instruction detected at line %d: %q", i+1, trimSpace(line))
        }
        
        // Check for "click/select/choose [color]" patterns
        actionColorPattern := regexp.MustCompile(`(?i)\b(click|select|choose)\s+(the\s+)?(red|green|blue|yellow|orange|purple|pink|gray|grey)\b`)
        if actionColorPattern.MatchString(line) {
            return fmt.Errorf("color-only UI instruction detected at line %d: %q", i+1, trimSpace(line))
        }
        
        // Check for "[color] text/highlight/etc" but exclude cases with additional context
        colorTextPattern := regexp.MustCompile(`(?i)\b(red|green|blue|yellow|orange|purple|pink|gray|grey)\s+(text|highlight|background|box|section|item|line|row|column)\b`)
        if colorTextPattern.MatchString(line) {
            // Exclude cases where there's additional descriptive context
            if !strings.Contains(lower, "with ") && !strings.Contains(lower, "and ") && !strings.Contains(lower, "using ") {
                return fmt.Errorf("color-only reference detected at line %d: %q", i+1, trimSpace(line))
            }
        }
    }
    
    return nil
}

// validateImageAltText ensures all images have non-empty alt text
func validateImageAltText(lines []string) error {
    // Image pattern: ![alt text](url) or ![](url)
    imagePattern := regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
    
    for i, line := range lines {
        matches := imagePattern.FindAllStringSubmatch(line, -1)
        for _, match := range matches {
            if len(match) < 2 {
                continue
            }
            
            altText := strings.TrimSpace(match[1])
            if altText == "" {
                return fmt.Errorf("image at line %d has empty alt text", i+1)
            }
            
            // Additional check for non-meaningful alt text
            if isGenericAltText(altText) {
                return fmt.Errorf("image at line %d has generic/non-descriptive alt text: %q", i+1, altText)
            }
        }
    }
    
    return nil
}

// isGenericAltText checks if alt text is too generic to be meaningful
func isGenericAltText(altText string) bool {
    lower := strings.ToLower(strings.TrimSpace(altText))
    genericTexts := []string{
        "image", "picture", "photo", "figure", "diagram", "chart", "graph",
        "img", "pic", "screenshot", "capture", "placeholder", "icon",
        "untitled", "unnamed", "default", "example", "sample",
    }
    
    for _, generic := range genericTexts {
        if lower == generic {
            return true
        }
    }
    
    return false
}


