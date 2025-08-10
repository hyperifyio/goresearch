package app

import (
    "regexp"
    "sort"
    "strings"
)

// appendGlossaryAppendix analyzes the provided markdown and, when it finds
// acronym definitions and/or frequently occurring key terms, appends a
// "Glossary" appendix as a Markdown section. When no entries are detected it
// returns the input unchanged.
func appendGlossaryAppendix(markdown string) string {
    // Avoid duplicating a glossary if one already exists
    if containsHeading(markdown, "glossary") {
        return markdown
    }

    // Work on the main body only; ignore references and other appendices
    body := sliceBeforeFirstHeading(markdown, []string{"references"})
    body = stripCodeFences(body)

    acronyms := extractAcronymDefinitions(body)
    terms := extractKeyTerms(body, 2)

    if len(acronyms) == 0 && len(terms) == 0 {
        return markdown
    }

    // Build appendix content
    var b strings.Builder
    b.WriteString(markdown)
    // Title will be prefixed with an Appendix label by the appendix manager.
    b.WriteString("\n\n## Glossary\n\n")

    // Acronyms first, sorted by key
    if len(acronyms) > 0 {
        keys := make([]string, 0, len(acronyms))
        for k := range acronyms { keys = append(keys, k) }
        sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
        for _, k := range keys {
            v := strings.TrimSpace(acronyms[k])
            if v != "" {
                b.WriteString("- ")
                b.WriteString(k)
                b.WriteString(" — ")
                b.WriteString(v)
                b.WriteString("\n")
            }
        }
        // Add a blank line between acronym and term blocks when both exist
        if len(terms) > 0 {
            b.WriteString("\n")
        }
    }

    // Key terms, sorted
    if len(terms) > 0 {
        keys := make([]string, 0, len(terms))
        for k := range terms {
            keys = append(keys, k)
        }
        sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
        for _, k := range keys {
            b.WriteString("- ")
            b.WriteString(k)
            b.WriteString("\n")
        }
    }

    return b.String()
}

// containsHeading returns true if markdown contains a heading whose text equals
// the provided title (case-insensitive).
func containsHeading(markdown, title string) bool {
    title = strings.TrimSpace(strings.ToLower(title))
    for _, line := range strings.Split(markdown, "\n") {
        s := trimInline(line)
        if !isMDHeading(s) { continue }
        if strings.EqualFold(stripMDHeading(s), title) {
            return true
        }
    }
    return false
}

// sliceBeforeFirstHeading returns the prefix of markdown appearing before the
// first heading whose text equals any of stopTitles (case-insensitive). When no
// stop heading is found, the full markdown is returned.
func sliceBeforeFirstHeading(markdown string, stopTitles []string) string {
    stop := map[string]struct{}{}
    for _, t := range stopTitles { stop[strings.ToLower(strings.TrimSpace(t))] = struct{}{} }
    lines := strings.Split(markdown, "\n")
    for i, line := range lines {
        s := trimInline(line)
        if !isMDHeading(s) { continue }
        if _, ok := stop[strings.ToLower(stripMDHeading(s))]; ok {
            return strings.Join(lines[:i], "\n")
        }
    }
    return markdown
}

func isMDHeading(s string) bool {
    if s == "" { return false }
    i := 0
    for i < len(s) && s[i] == '#' { i++ }
    return i > 0 && i <= 6 && (i < len(s) && s[i] == ' ')
}

func stripMDHeading(s string) string {
    i := 0
    for i < len(s) && s[i] == '#' { i++ }
    if i < len(s) && s[i] == ' ' { i++ }
    return strings.TrimSpace(s[i:])
}

func trimInline(s string) string {
    i := 0
    j := len(s)
    for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') { i++ }
    for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') { j-- }
    return s[i:j]
}

// stripCodeFences removes fenced code blocks (``` ... ```), leaving other content.
func stripCodeFences(markdown string) string {
    lines := strings.Split(markdown, "\n")
    var out []string
    inFence := false
    for _, line := range lines {
        s := strings.TrimSpace(line)
        if strings.HasPrefix(s, "```") {
            inFence = !inFence
            continue
        }
        if inFence { continue }
        out = append(out, line)
    }
    return strings.Join(out, "\n")
}

// extractAcronymDefinitions finds patterns of the form "Long Form (ACRO)" or
// "ACRO (Long Form)" and returns a map from acronym to long form.
func extractAcronymDefinitions(text string) map[string]string {
    // Limit long form to 1..7 words comprised of letters, digits, and a few separators
    // Pattern A: Long form (ACRO)
    reA := regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9&/\-]+(?:\s+[A-Za-z][A-Za-z0-9&/\-]+){0,6})\s*\(([A-Za-z]{2,6})\)`)
    // Pattern B: ACRO (Long form)
    reB := regexp.MustCompile(`\b([A-Za-z]{2,6})\s*\(([A-Za-z][A-Za-z0-9&/\-]+(?:\s+[A-Za-z][A-Za-z0-9&/\-]+){0,6})\)`)

    out := map[string]string{}

    // Helper to normalize long form: collapse spaces, trim trailing punctuation
    normalize := func(s string) string {
        s = strings.TrimSpace(s)
        s = strings.Trim(s, ":;,. ")
        // Avoid capturing all-caps phrases (should be a proper term)
        if s == strings.ToUpper(s) { return s }
        // Collapse inner whitespace
        fields := strings.Fields(s)
        return strings.Join(fields, " ")
    }

    for _, m := range reA.FindAllStringSubmatch(text, -1) {
        long := normalize(m[1])
        // For pattern A (Long form (ACRO)), prefer the trailing sequence of
        // title-cased words nearest the parentheses to avoid capturing
        // preceding filler like "is compared to".
        long = trailingTitleCasedSequence(long)
        acro := m[2]
        if isReasonableLongForm(long) {
            if _, exists := out[acro]; !exists {
                out[acro] = long
            }
        }
    }
    for _, m := range reB.FindAllStringSubmatch(text, -1) {
        acro := m[1]
        long := normalize(m[2])
        if isReasonableLongForm(long) {
            if _, exists := out[acro]; !exists {
                out[acro] = long
            }
        }
    }
    return out
}

func isReasonableLongForm(s string) bool {
    if s == "" { return false }
    // At least two alphabetic characters total
    letters := 0
    for _, r := range s {
        if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') { letters++ }
        if letters >= 2 { break }
    }
    if letters < 2 { return false }
    // Word count 1..7
    wc := len(strings.Fields(s))
    return wc >= 1 && wc <= 7
}

// trailingTitleCasedSequence returns the longest contiguous sequence of 1..4
// title-cased words at the end of the phrase. If none are found, the original
// string is returned.
func trailingTitleCasedSequence(s string) string {
    words := strings.Fields(s)
    if len(words) == 0 { return s }
    end := len(words) - 1
    start := end
    count := 0
    for i := end; i >= 0; i-- {
        w := words[i]
        if isTitleWord(w) {
            start = i
            count++
            if count >= 4 { break }
            continue
        }
        // stop when a non-title word is encountered after collecting some
        if count > 0 { break }
    }
    if count == 0 { return s }
    return strings.Join(words[start:end+1], " ")
}

func isTitleWord(w string) bool {
    if len(w) == 0 { return false }
    r := rune(w[0])
    if !(r >= 'A' && r <= 'Z') { return false }
    // require at least one lowercase letter somewhere (to exclude ALLCAPS)
    hasLower := false
    for _, ch := range w { if ch >= 'a' && ch <= 'z' { hasLower = true; break } }
    return hasLower
}

// extractKeyTerms returns title-cased multi-word terms that appear at least minCount
// times in the text. It ignores headings, code fences (already stripped), and
// a small set of common phrases.
func extractKeyTerms(text string, minCount int) map[string]int {
    // Title-cased 2..4 word phrases
    re := regexp.MustCompile(`\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+){1,3})\b`)
    counts := map[string]int{}
    firstCased := map[string]string{}
    lowerStop := map[string]struct{}{
        "executive summary": {},
        "risks and limitations": {},
        "related work": {},
        "future work": {},
        "introduction": {},
        "conclusion": {},
        // months
        "january": {}, "february": {}, "march": {}, "april": {}, "may": {}, "june": {},
        "july": {}, "august": {}, "september": {}, "october": {}, "november": {}, "december": {},
    }
    for _, m := range re.FindAllStringSubmatch(text, -1) {
        phrase := strings.TrimSpace(m[1])
        low := strings.ToLower(phrase)
        if _, stop := lowerStop[low]; stop { continue }
        // filter very short words-only pairs like "The Report"
        words := strings.Fields(phrase)
        if len(words) < 2 || len(words) > 4 { continue }
        // Heuristic: skip if any word is <=2 letters (common function words)
        short := false
        for _, w := range words { if len(w) <= 2 { short = true; break } }
        if short { continue }
        key := strings.ToLower(strings.Join(words, " "))
        counts[key]++
        if _, ok := firstCased[key]; !ok { firstCased[key] = strings.Join(words, " ") }
    }
    out := map[string]int{}
    for k, c := range counts { if c >= minCount { out[firstCased[k]] = c } }
    return out
}
