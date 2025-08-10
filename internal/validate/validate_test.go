package validate

import (
    "strings"
    "testing"
    "time"
)

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

// Tests for FEATURE_CHECKLIST item 287: Reference quality/mix validator — configurable policy
// to prefer peer-reviewed/standards, ensure recency where appropriate, and prevent over-reliance.
func TestValidateReferenceQuality_AtLeastOnePreferred(t *testing.T) {
    md := `# Title

## References
1. Blog A — https://blog.example.com/post/alpha (2012)
2. Blog B — https://blog.example.com/post/bravo (2014)
3. W3C Spec — https://www.w3.org/TR/something/ (2016)
4. arXiv Paper — https://arxiv.org/abs/2401.12345 (2024)
5. Generic — https://example.com/info (2025)
`
    pol := ReferenceQualityPolicy{
        RequireAtLeastOnePreferred: true,
        PreferredHostPatterns:      []string{"w3.org", "arxiv.org", "whatwg.org", "rfc-editor.org"},
    }
    if err := ValidateReferenceQuality(md, pol); err != nil {
        t.Fatalf("expected to pass with at least one preferred, got error: %v", err)
    }

    // Remove preferred sources to force failure
    md2 := strings.ReplaceAll(md, "https://www.w3.org/TR/something/ (2016)", "https://example.net/spec (2016)")
    md2 = strings.ReplaceAll(md2, "https://arxiv.org/abs/2401.12345 (2024)", "https://another.example/paper (2024)")
    if err := ValidateReferenceQuality(md2, pol); err == nil {
        t.Fatalf("expected failure when no preferred sources are present")
    }
}

func TestValidateReferenceQuality_MinPreferredFraction(t *testing.T) {
    md := `# T

## References
1. Blog A — https://blog.example.com/a (2012)
2. Blog B — https://blog.example.com/b (2014)
3. W3C — https://www.w3.org/TR/x/ (2016)
4. arXiv — https://arxiv.org/abs/2401.1 (2024)
5. Generic — https://example.com/info (2025)
`
    polOK := ReferenceQualityPolicy{
        MinPreferredFraction:   0.40, // 2/5 = 0.4 OK
        PreferredHostPatterns:  []string{"w3.org", "arxiv.org"},
    }
    if err := ValidateReferenceQuality(md, polOK); err != nil {
        t.Fatalf("expected preferred fraction to pass, got error: %v", err)
    }

    polFail := polOK
    polFail.MinPreferredFraction = 0.50 // 2/5 < 0.5
    if err := ValidateReferenceQuality(md, polFail); err == nil {
        t.Fatalf("expected failure when preferred fraction is below threshold")
    }
}

func TestValidateReferenceQuality_MaxPerDomainCaps(t *testing.T) {
    md := `# T

## References
1. Blog A — https://blog.example.com/a (2012)
2. Blog B — https://blog.example.com/b (2014)
3. Blog C — https://blog.example.com/c (2016)
4. Other — https://other.example/d (2024)
5. Other2 — https://other.example/e (2025)
`
    // Absolute cap
    polAbs := ReferenceQualityPolicy{MaxPerDomain: 2}
    if err := ValidateReferenceQuality(md, polAbs); err == nil {
        t.Fatalf("expected failure: blog.example.com appears 3 times > cap 2")
    }

    // Fractional cap: blog.example.com is 3/5=0.6
    polFrac := ReferenceQualityPolicy{MaxPerDomainFraction: 0.50}
    if err := ValidateReferenceQuality(md, polFrac); err == nil {
        t.Fatalf("expected failure: domain fraction exceeds 0.50")
    }

    // Looser fraction should pass
    polFracOK := ReferenceQualityPolicy{MaxPerDomainFraction: 0.60}
    if err := ValidateReferenceQuality(md, polFracOK); err != nil {
        t.Fatalf("expected pass at 0.60 (strictly greater check), got %v", err)
    }
}

func TestValidateReferenceQuality_RecencyFractionWithExempt(t *testing.T) {
    md := `# T

## References
1. Blog A — https://blog.example.com/a (2012)
2. Blog B — https://blog.example.com/b (2014)
3. W3C — https://www.w3.org/TR/x/ (2016)
4. arXiv — https://arxiv.org/abs/2401.1 (2024)
5. Generic — https://example.com/info (2025)
`
    fixedNow := func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
    pol := ReferenceQualityPolicy{
        RecentWithinYears:        5,     // cutoff 2020
        MinRecentFraction:        0.50,  // need at least 2/4 recent (excluding exempt)
        RecencyExemptHostPatterns: []string{"w3.org"},
        Now:                      fixedNow,
    }
    // Excluding w3.org from denominator: recent are 2024 and 2025 => 2/4 = 0.5 OK
    if err := ValidateReferenceQuality(md, pol); err != nil {
        t.Fatalf("expected recency check to pass with exemption, got error: %v", err)
    }

    polFail := pol
    polFail.MinRecentFraction = 0.60 // 0.5 < 0.6 -> should fail
    if err := ValidateReferenceQuality(md, polFail); err == nil {
        t.Fatalf("expected recency fraction failure at 0.60 threshold")
    }
}

func TestValidateReferenceQuality_RecencyAllExemptVacuousPass(t *testing.T) {
    md := `# T

## References
1. W3C — https://www.w3.org/TR/x/ (2010)
2. RFC — https://www.rfc-editor.org/rfc/rfc9110 (2018)
`
    fixedNow := func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
    pol := ReferenceQualityPolicy{
        RecentWithinYears:         5,
        MinRecentFraction:         0.80, // would fail if counted
        RecencyExemptHostPatterns: []string{"w3.org", "rfc-editor.org"},
        Now:                       fixedNow,
    }
    if err := ValidateReferenceQuality(md, pol); err != nil {
        t.Fatalf("expected vacuous pass when all hosts are exempt, got %v", err)
    }
}


