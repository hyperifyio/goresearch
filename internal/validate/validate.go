package validate

import (
    "fmt"
    "regexp"
    "sort"
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
    c := ValidateCitations(markdown, n)
    if c.MissingReferences {
        return fmt.Errorf("citations present but no references")
    }
    if len(c.OutOfRange) > 0 {
        return fmt.Errorf("out-of-range citations: %v", c.OutOfRange)
    }
    return nil
}


