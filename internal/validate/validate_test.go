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


