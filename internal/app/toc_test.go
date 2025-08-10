package app

import (
    "strings"
    "testing"
)

func TestAppendAutoToC_ThresholdAndAnchors(t *testing.T) {
    md := "# Title\n2025-01-01\nAuthor: Jane\nVersion: v1.2.3\n\n## Sec 1\nText\n\n## Sec 2\nText\n\n## Sec 3\nText\n\n## Sec 4\nText\n\n## Sec 5\nText\n\n## Sec 6\nText\n\n## Sec 7\nText\n\n## References\n1. A — https://a\n"
    // 7 H2s < 12 default threshold -> unchanged
    out := appendAutoToC(md, 12)
    if strings.Contains(out, "Table of contents") {
        t.Fatalf("unexpected ToC when below threshold")
    }
    // Force threshold to 5 -> ToC appears
    out2 := appendAutoToC(md, 5)
    if !strings.Contains(out2, "## Table of contents") {
        t.Fatalf("expected ToC inserted")
    }
    if !strings.Contains(out2, "](#sec-1)") || !strings.Contains(out2, "](#sec-7)") {
        t.Fatalf("expected anchor slugs for headings:\n%s", out2)
    }
    // Idempotent: calling again should not duplicate ToC
    out3 := appendAutoToC(out2, 5)
    if strings.Count(out3, "## Table of contents") != 1 {
        t.Fatalf("expected single ToC after repeated calls")
    }
}

func TestAppendAutoToC_InsertionAfterHeader(t *testing.T) {
    md := "# Title\n2025-01-01\nAuthor: John Doe\nVersion: 1.0.0\n\n## A\nX\n\n## B\nY\n\n## C\nZ\n\n## D\n\n## E\n\n## F\n\n## G\n\n## H\n\n## I\n\n## J\n\n## K\n\n## L\n"
    out := appendAutoToC(md, 4)
    // ToC should appear before first section heading and after metadata lines
    // i.e., after Version line
    idx := strings.Index(out, "## Table of contents")
    metaIdx := strings.Index(out, "Version: 1.0.0")
    secIdx := strings.Index(out, "## A")
    if !(idx > metaIdx && idx < secIdx) {
        t.Fatalf("toc insertion position unexpected")
    }
}

func TestManageAppendices_LabelAndReference(t *testing.T) {
    md := `# Title
2025-01-01

## Body
Text.

## References
1. A — https://a.example

## Evidence check
Summary.

## Glossary
- Term — Definition
`
    out := manageAppendices(md)
    if !strings.Contains(strings.ToLower(out), "## appendix a. evidence check") {
        t.Fatalf("expected Appendix A label for Evidence check; got:\n%s", out)
    }
    if !strings.Contains(strings.ToLower(out), "## appendix b. glossary") {
        t.Fatalf("expected Appendix B label for Glossary; got:\n%s", out)
    }
    if !strings.Contains(strings.ToLower(out), "see appendices:") {
        t.Fatalf("expected body reference to appendices; got:\n%s", out)
    }
}
