package app

import (
    "strings"
)

// manageAppendices auto-labels appendix sections with sequential letters (A, B, C, ...)
// and ensures each is referenced from the body with anchor links. Appendices are
// detected as H2 sections appearing after the "## References" heading whose titles
// match known appendix types (Evidence check, Glossary, Manifest). Idempotent.
func manageAppendices(markdown string) string {
    md := strings.ReplaceAll(markdown, "\r\n", "\n")
    lines := strings.Split(md, "\n")

    // Find the line index of the References section (case-insensitive)
    refIdx := -1
    for i, raw := range lines {
        s := strings.TrimSpace(raw)
        if strings.HasPrefix(s, "## ") && strings.EqualFold(strings.TrimSpace(s[3:]), "references") {
            refIdx = i
            break
        }
    }
    if refIdx == -1 {
        // No references: nothing to manage
        return markdown
    }

    type appx struct{ line int; baseTitle string; baseNorm string }
    var found []appx

    // Scan headings after References for appendix candidates
    for i := refIdx + 1; i < len(lines); i++ {
        s := strings.TrimSpace(lines[i])
        if !strings.HasPrefix(s, "## ") { continue }
        title := strings.TrimSpace(s[3:])
        // Normalize by stripping an existing "Appendix X. " prefix if present
        low := strings.ToLower(title)
        if strings.HasPrefix(low, "appendix ") {
            // find ". " separator
            if dot := strings.Index(title, ". "); dot != -1 {
                title = strings.TrimSpace(title[dot+2:])
                low = strings.ToLower(title)
            }
        }
        // Match known appendix base titles
        if strings.HasPrefix(low, "evidence check") || strings.EqualFold(low, "glossary") || strings.EqualFold(low, "manifest") {
            baseNorm := low
            // Collapse variants like "evidence" to "evidence check"
            if strings.HasPrefix(baseNorm, "evidence") { baseNorm = "evidence check" }
            found = append(found, appx{line: i, baseTitle: title, baseNorm: baseNorm})
        }
    }

    if len(found) == 0 {
        return markdown
    }

    // Assign letters A, B, C... using a stable, user-centric order:
    // Evidence check, Glossary, then Manifest, based on presence.
    assigned := map[string]rune{}
    present := map[string]bool{}
    for _, ap := range found { present[ap.baseNorm] = true }
    order := make([]string, 0, 3)
    if present["evidence check"] { order = append(order, "evidence check") }
    if present["glossary"] { order = append(order, "glossary") }
    if present["manifest"] { order = append(order, "manifest") }
    for idx, key := range order {
        assigned[key] = rune('A' + idx)
    }
    // Rewrite headings with assigned letters, keeping duplicates consistent
    for i := range found {
        letter := assigned[found[i].baseNorm]
        newTitle := "Appendix " + string(letter) + ". " + found[i].baseTitle
        lines[found[i].line] = "## " + newTitle
        found[i].baseTitle = newTitle
    }

    // Ensure a body reference line exists before References
    // Use a deterministic marker to avoid duplicates
    markerPrefix := "See appendices: "
    already := false
    for i := 0; i < refIdx; i++ {
        if strings.Contains(lines[i], markerPrefix) { already = true; break }
    }
    if !already {
        // Build inline list with anchor links; include user-facing appendices only
        var b strings.Builder
        b.WriteString(markerPrefix)
        wrote := 0
        used := map[string]bool{}
        for _, key := range order {
            // Skip manifest in body references
            if key == "manifest" { continue }
            if used[key] { continue }
            used[key] = true
            // construct composite title for this appendix label
            letter := assigned[key]
            var base string
            if key == "evidence check" { base = "Evidence check" } else { base = strings.Title(key) }
            full := "Appendix " + string(letter) + ". " + base
            if wrote > 0 { b.WriteString("; ") }
            wrote++
            anchor := makeSlugForAnchor(full)
            b.WriteString("[")
            b.WriteString(full)
            b.WriteString("](#")
            b.WriteString(anchor)
            b.WriteString(")")
        }
        // Insert the reference line after the References section block
        insertAt := len(lines)
        for i := refIdx + 1; i < len(lines); i++ {
            s := strings.TrimSpace(lines[i])
            if strings.HasPrefix(s, "## ") { insertAt = i; break }
        }
        out := make([]string, 0, len(lines)+2)
        out = append(out, lines[:insertAt]...)
        if insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) != "" {
            out = append(out, "")
        }
        out = append(out, b.String())
        out = append(out, "")
        out = append(out, lines[insertAt:]...)
        lines = out
    }

    return strings.Join(lines, "\n")
}

// makeSlugForAnchor mirrors the ToC slug rules to generate in-document anchors.
func makeSlugForAnchor(s string) string {
    s = strings.ToLower(strings.TrimSpace(s))
    var b strings.Builder
    lastHyphen := false
    for _, r := range s {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
            b.WriteRune(r)
            lastHyphen = false
            continue
        }
        if r == ' ' || r == '-' || r == '_' || r == '.' {
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
