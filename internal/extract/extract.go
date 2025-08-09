package extract

import (
    "bytes"
    "strings"

    "golang.org/x/net/html"
)

// Document is a simplified representation of extracted page content.
type Document struct {
    Title string
    Text  string
}

// FromHTML extracts readable text from HTML, preferring <main> or <article>,
// falling back to <body>. It preserves headings, paragraphs, list items,
// and pre/code blocks, while skipping obvious boilerplate like <nav> and <footer>.
func FromHTML(input []byte) Document {
    node, err := html.Parse(bytes.NewReader(input))
    if err != nil || node == nil {
        return Document{}
    }

    title := strings.TrimSpace(findTitle(node))
    // Pick content root
    var content *html.Node
    content = findFirst(node, "main")
    if content == nil {
        content = findFirst(node, "article")
    }
    if content == nil {
        content = findFirst(node, "body")
    }
    var b strings.Builder
    if content != nil {
        // Walk and collect text with simple heuristics
        collectText(&b, content, false)
    }
    // post-process: collapse whitespace and remove many blank lines
    text := normalizeWhitespace(b.String())
    return Document{Title: title, Text: text}
}

func findTitle(n *html.Node) string {
    head := findFirst(n, "head")
    if head == nil {
        return ""
    }
    t := findFirst(head, "title")
    if t == nil || t.FirstChild == nil {
        return ""
    }
    return t.FirstChild.Data
}

func findFirst(n *html.Node, tag string) *html.Node {
    var res *html.Node
    var dfs func(*html.Node)
    dfs = func(cur *html.Node) {
        if res != nil {
            return
        }
        if cur.Type == html.ElementNode && strings.EqualFold(cur.Data, tag) {
            res = cur
            return
        }
        for c := cur.FirstChild; c != nil; c = c.NextSibling {
            dfs(c)
            if res != nil {
                return
            }
        }
    }
    dfs(n)
    return res
}

func collectText(b *strings.Builder, n *html.Node, inPre bool) {
    if n.Type == html.ElementNode {
        // Skip known boilerplate containers like cookie/consent banners
        if isBoilerplateContainer(n) {
            return
        }
        name := strings.ToLower(n.Data)
        switch name {
        case "script", "style", "noscript", "nav", "footer", "aside", "iframe":
            return
        case "pre", "code":
            inPre = true
        case "br", "hr":
            b.WriteString("\n")
        case "p", "h1", "h2", "h3", "h4", "h5", "h6", "li":
            // Add a newline before block starts to ensure separation
            b.WriteString("\n")
        case "ul", "ol":
            // group items with newlines
            b.WriteString("\n")
        }
    }

    switch n.Type {
    case html.TextNode:
        data := n.Data
        if !inPre {
            data = strings.ReplaceAll(data, "\t", " ")
            data = strings.ReplaceAll(data, "\r", " ")
        }
        b.WriteString(data)
    }

    for c := n.FirstChild; c != nil; c = c.NextSibling {
        collectText(b, c, inPre)
    }

    if n.Type == html.ElementNode {
        name := strings.ToLower(n.Data)
        switch name {
        case "p", "h1", "h2", "h3", "h4", "h5", "h6":
            b.WriteString("\n\n")
        case "li":
            b.WriteString("\n")
        case "pre", "code":
            inPre = false
            b.WriteString("\n")
        }
    }
}

// isBoilerplateContainer returns true if the element looks like a cookie/consent banner.
func isBoilerplateContainer(n *html.Node) bool {
    if n == nil || n.Type != html.ElementNode {
        return false
    }
    // Check id and class attributes for common markers
    for _, attr := range n.Attr {
        key := strings.ToLower(attr.Key)
        if key != "id" && key != "class" && !strings.HasPrefix(key, "data-") && key != "aria-label" && key != "role" {
            continue
        }
        val := strings.ToLower(attr.Val)
        if containsAny(val, []string{"cookie", "consent", "gdpr"}) {
            return true
        }
        // Common banner/toast/modal hints when combined with consent markers often appear on ancestors.
        if containsAny(val, []string{"cookie-banner", "cookiebar", "consent-banner", "consent-manager"}) {
            return true
        }
    }
    return false
}

func containsAny(s string, needles []string) bool {
    for _, n := range needles {
        if strings.Contains(s, n) {
            return true
        }
    }
    return false
}

func normalizeWhitespace(s string) string {
    // Collapse multiple spaces and blank lines
    lines := strings.Split(s, "\n")
    out := make([]string, 0, len(lines))
    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        if trimmed == "" {
            // Keep at most one consecutive blank
            if len(out) > 0 && out[len(out)-1] == "" {
                continue
            }
            out = append(out, "")
            continue
        }
        // collapse internal whitespace runs to single spaces
        collapsed := collapseSpaces(trimmed)
        out = append(out, collapsed)
    }
    // trim trailing blank line
    for len(out) > 0 && out[len(out)-1] == "" {
        out = out[:len(out)-1]
    }
    return strings.Join(out, "\n")
}

func collapseSpaces(s string) string {
    var b strings.Builder
    lastSpace := false
    for _, r := range s {
        if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
            if !lastSpace {
                b.WriteByte(' ')
                lastSpace = true
            }
            continue
        }
        b.WriteRune(r)
        lastSpace = false
    }
    return b.String()
}


