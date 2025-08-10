package app

import (
    "bufio"
    "regexp"
    "strings"

    "github.com/jung-kurt/gofpdf"
)

// writeSimplePDF renders a minimal PDF from Markdown text, preserving paragraphs and
// turning Markdown links [text](url) into clickable PDF links. This is intentionally
// simple and does not perform full Markdown layout.
func writeSimplePDF(markdown string, outPath string) error {
    pdf := gofpdf.New("P", "mm", "A4", "")
    pdf.SetFont("Helvetica", "", 11)
    pdf.AddPage()

    linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`) // [text](url)

    // Render line by line to avoid huge paragraphs
    scanner := bufio.NewScanner(strings.NewReader(markdown))
    scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
    for scanner.Scan() {
        line := scanner.Text()
        s := strings.TrimSpace(line)
        if s == "" {
            pdf.Ln(5)
            continue
        }
        // Strip heading markers for a basic layout, but add spacing
        if strings.HasPrefix(s, "#") {
            // Count leading '#'
            i := 0
            for i < len(s) && s[i] == '#' { i++ }
            text := strings.TrimSpace(s[i:])
            if text == "" { continue }
            // Larger font for headings
            size := 14.0
            if i >= 2 { size = 12.0 }
            pdf.SetFont("Helvetica", "B", size)
            pdf.CellFormat(0, 8, text, "", 1, "L", false, 0, "")
            pdf.SetFont("Helvetica", "", 11)
            continue
        }
        // Replace markdown links inline by writing text segments and links
        parts := linkRe.FindAllStringSubmatchIndex(s, -1)
        if len(parts) == 0 {
            pdf.MultiCell(0, 5, s, "", "L", false)
            continue
        }
        pos := 0
        for _, m := range parts {
            // m: [fullStart, fullEnd, textStart, textEnd, urlStart, urlEnd]
            if m[0] > pos {
                pdf.Write(5, s[pos:m[0]])
            }
            text := s[m[2]:m[3]]
            url := s[m[4]:m[5]]
            if strings.HasPrefix(url, "#") {
                // Intra-doc anchors: render as plain text in PDF for now
                pdf.Write(5, text)
            } else {
                pdf.WriteLinkString(5, text, url)
            }
            pos = m[1]
        }
        if pos < len(s) {
            pdf.Write(5, s[pos:])
        }
        pdf.Ln(6)
    }

    // Write file
    return pdf.OutputFileAndClose(outPath)
}
