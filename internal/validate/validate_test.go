package validate

import "testing"

func TestValidateReferencesCompleteness_OK(t *testing.T) {
    md := `# Title

## References
1. Example Domain — https://example.com
2. [Go net/http](https://pkg.go.dev/net/http) — https://pkg.go.dev/net/http
`
    bad := ValidateReferencesCompleteness(md)
    if len(bad) != 0 {
        t.Fatalf("expected all references complete, got bad indices %v", bad)
    }
}

func TestValidateReferencesCompleteness_MissingURL(t *testing.T) {
    md := `# Title

## References
1. Example Domain
2. Another — https://example.com
`
    bad := ValidateReferencesCompleteness(md)
    if len(bad) != 1 || bad[0] != 1 {
        t.Fatalf("expected item 1 incomplete, got %v", bad)
    }
}

func TestValidateReferencesCompleteness_MissingTitle(t *testing.T) {
    md := `# Title

## References
1. https://example.com
2. RFC 9110 — https://www.rfc-editor.org/rfc/rfc9110
`
    bad := ValidateReferencesCompleteness(md)
    if len(bad) != 1 || bad[0] != 1 {
        t.Fatalf("expected item 1 incomplete (no title), got %v", bad)
    }
}

func TestValidateReport_IncompleteReferencesFails(t *testing.T) {
    md := `# Title
2025-01-01

Body with a cite [1].

## References
1. https://example.com
`
    if err := ValidateReport(md); err == nil {
        t.Fatalf("expected validation error for incomplete references")
    }
}

func TestValidateStructure_OK_WithOutline(t *testing.T) {
    md := `# Title
2025-01-01

## Executive summary
Some text.

## Background
Info.

## Risks and limitations
Notes.

## References
1. Example — https://example.com`
    outline := []string{"Executive summary", "Background"}
    if err := ValidateStructure(md, outline); err != nil {
        t.Fatalf("expected structure to be valid, got error: %v", err)
    }
}

func TestValidateStructure_MissingTitle(t *testing.T) {
    md := `Not a heading title
2025-01-01

## References
1. Example — https://example.com`
    if err := ValidateStructure(md, nil); err == nil {
        t.Fatalf("expected missing H1 title to be an error")
    }
}

func TestValidateStructure_MissingDate(t *testing.T) {
    md := `# Title

## References
1. Example — https://example.com`
    if err := ValidateStructure(md, nil); err == nil {
        t.Fatalf("expected missing date to be an error")
    }
}

func TestValidateStructure_MissingRisks(t *testing.T) {
    md := `# Title
2025-01-01

## Executive summary
Text.

## References
1. Example — https://example.com`
    if err := ValidateStructure(md, []string{"Executive summary"}); err == nil {
        t.Fatalf("expected missing risks section to be an error")
    }
}

func TestValidateStructure_OutlineOrder(t *testing.T) {
    md := `# Title
2025-01-01

## Background
Info.

## Executive summary
Some.

## Risks and limitations
Notes.

## References
1. Example — https://example.com`
    outline := []string{"Executive summary", "Background"}
    if err := ValidateStructure(md, outline); err == nil {
        t.Fatalf("expected out-of-order outline to be an error")
    }
}

func TestValidateStructure_RejectExtraH1(t *testing.T) {
    md := `# Title
2025-01-01

# Another H1
Text.

## Risks and limitations
Notes.

## References
1. Example — https://example.com`
    if err := ValidateStructure(md, nil); err == nil {
        t.Fatalf("expected extra H1 to be rejected")
    }
}


