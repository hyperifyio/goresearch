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

## Alternatives & conflicting evidence
Short counter-evidence summary.

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

## Alternatives & conflicting evidence
Short.

## References
1. Example — https://example.com`
    if err := ValidateStructure(md, []string{"Executive summary"}); err == nil {
        t.Fatalf("expected missing risks section to be an error")
    }
}

func TestValidateStructure_MissingAlternativesConflicting(t *testing.T) {
    md := `# Title
2025-01-01

## Executive summary
Text.

## Risks and limitations
Notes.

## References
1. Example — https://example.com`
    if err := ValidateStructure(md, []string{"Executive summary"}); err == nil {
        t.Fatalf("expected missing Alternatives & conflicting evidence section to be an error")
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

// Tests for FEATURE_CHECKLIST item 277: Audience fit check — per-brief audience/tone settings
// and a pass that flags jargon or sections mismatched to the intended reader.
func TestValidateAudienceFit_NonTechnical_JargonAndCode_Fails(t *testing.T) {
    var sb strings.Builder
    sb.WriteString("# Title\n")
    sb.WriteString("2025-01-01\n\n")
    sb.WriteString("## Executive summary\n")
    sb.WriteString("This executive brief explains the approach.\n\n")
    sb.WriteString("## Implementation details\n")
    sb.WriteString("We implement a gRPC API with JWT, OAuth, and schema serialization using Protobuf. The algorithm ensures eventual consistency and uses sharding and quorum-based consensus like Raft.\n\n")
    sb.WriteString("Example:\n")
    sb.WriteString("```\n")
    sb.WriteString("curl -H \"Authorization: Bearer ...\" https://api.example.com/v1/resource\n")
    sb.WriteString("```\n\n")
    sb.WriteString("## Risks and limitations\n")
    sb.WriteString("None.\n\n")
    sb.WriteString("## References\n")
    sb.WriteString("1. Example — https://example.com")
    md := sb.String()
    // Non-technical audience should trigger issues due to technical section title,
    // jargon density, and presence of code block.
    if err := ValidateAudienceFit(md, "Executive, non-technical stakeholders", "formal"); err == nil {
        t.Fatalf("expected audience fit issues for non-technical brief with jargon and code")
    }
}

func TestValidateAudienceFit_FormalTone_CasualMarkers_Fails(t *testing.T) {
    md := `# T
2025-01-01

## Executive summary
This is a super cool overview! It kinda shows what's awesome about the thing.

## Risks and limitations
OK.

## References
1. Example — https://example.com`
    if err := ValidateAudienceFit(md, "engineers", "formal"); err == nil {
        t.Fatalf("expected tone issues for formal tone with casual markers")
    }
}

func TestValidateAudienceFit_TechnicalAudience_AllowsJargon_OK(t *testing.T) {
    md := `# T
2025-01-01

## Background
We evaluate latency, throughput, and idempotent retry semantics under strong consistency.

## Risks and limitations
N/A

## References
1. Example — https://example.com`
    if err := ValidateAudienceFit(md, "Senior engineers and architects", "concise technical"); err != nil {
        t.Fatalf("expected no audience fit issues for technical audience, got %v", err)
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


// Tests for FEATURE_CHECKLIST item 281: Visuals QA — numbered figures/tables with captions,
// required in-text references ("See Fig. X"), alt text, and placement near discussion.
func TestValidateVisuals_NoVisuals_OK(t *testing.T) {
    md := `# Title
2025-01-01

Some text.
`
    if err := ValidateVisuals(md); err != nil {
        t.Fatalf("expected no error when no visuals present, got %v", err)
    }
}

func TestValidateVisuals_Figure_OK(t *testing.T) {
    md := `# T
2025-01-01

See Fig. 1 for an overview.

![Figure 1: Overview of the system](image.png)

Figure 1: Overview of the system`
    if err := ValidateVisuals(md); err != nil {
        t.Fatalf("expected figure to pass visuals validation, got %v", err)
    }
}

func TestValidateVisuals_Figure_MissingAlt_Fails(t *testing.T) {
    md := `# T
2025-01-01

See Fig. 1 for context.

![](image.png)

Figure 1: Caption`
    if err := ValidateVisuals(md); err == nil {
        t.Fatalf("expected error for missing alt text on figure")
    }
}

func TestValidateVisuals_Figure_NoMention_Fails(t *testing.T) {
    md := `# T
2025-01-01

Some text without referencing the figure.

![Figure 1: Caption](img.png)

Figure 1: Caption`
    if err := ValidateVisuals(md); err == nil {
        t.Fatalf("expected error when figure is never referenced in text")
    }
}

func TestValidateVisuals_Figure_NumberingGap_Fails(t *testing.T) {
    md := `# T
2025-01-01

See Fig. 1 and Fig. 3.

![Figure 1: One](a.png)
Figure 1: One

![Figure 3: Three](c.png)
Figure 3: Three`
    if err := ValidateVisuals(md); err == nil {
        t.Fatalf("expected error for non-sequential figure numbering")
    }
}

func TestValidateVisuals_Table_OK(t *testing.T) {
    md := `# T
2025-01-01

See Table 1 for metrics.

Table 1: Metrics by category

| Col A | Col B |
| ----- | ----- |
| 1     | 2     |`
    if err := ValidateVisuals(md); err != nil {
        t.Fatalf("expected table to pass visuals validation, got %v", err)
    }
}

func TestValidateVisuals_Table_ReferenceTooFar_Fails(t *testing.T) {
    var sb strings.Builder
    sb.WriteString("# T\n2025-01-01\n\n")
    // Mention at top
    sb.WriteString("See Table 1 for details.\n\n")
    // Add many spacer lines to exceed ±8 window
    for i := 0; i < 15; i++ { sb.WriteString("Spacer line\n") }
    // Table at bottom with caption
    sb.WriteString("\n| A | B |\n|---|---|\n| x | y |\n\n")
    sb.WriteString("Table 1: Details caption\n")
    if err := ValidateVisuals(sb.String()); err == nil {
        t.Fatalf("expected error for table reference too far from table")
    }
}

// Tests for FEATURE_CHECKLIST item 295: “Ready for distribution” checks — metadata and anchor links
func TestValidateDistributionReady_OK(t *testing.T) {
    md := `# Report Title

2025-01-01

Author: Jane Doe
Version: v1.2.3

## Intro
See [details](#risks-and-limitations).

## Risks and limitations
Text.

## References
1. A — https://a.example`
    if err := ValidateDistributionReady(md, "Jane Doe", "v1.2.3"); err != nil {
        t.Fatalf("unexpected distribution-ready error: %v", err)
    }
}

func TestValidateDistributionReady_MissingAuthor(t *testing.T) {
    md := `# T

2025-01-01

Version: v0.1.0

## References
1. A — https://a`
    if err := ValidateDistributionReady(md, "", ""); err == nil {
        t.Fatalf("expected missing author error")
    }
}

func TestValidateDistributionReady_BrokenAnchor(t *testing.T) {
    md := `# T

2025-01-01

Author: X
Version: v0.1.0

See [foo](#no-such-heading).

## References
1. A — https://a`
    if err := ValidateDistributionReady(md, "", ""); err == nil {
        t.Fatalf("expected broken anchor error")
    }
}


// Tests for FEATURE_CHECKLIST item 271: Title quality check — enforce ≤12 words,
// descriptive keywords, and no unexplained acronyms/jargon.
func TestValidateTitleQuality_WordLimit(t *testing.T) {
    md := `# This Title Has More Than Twelve Words In It For Sure Indeed Today
2025-01-01

## References
1. A — https://a`
    if err := ValidateTitleQuality(md); err == nil {
        t.Fatalf("expected failure for title exceeding 12 words")
    }
}

func TestValidateTitleQuality_KeywordsRequired(t *testing.T) {
    md := `# A Study Of The Web
2025-01-01

## References
1. A — https://a`
    if err := ValidateTitleQuality(md); err == nil {
        t.Fatalf("expected failure for lacking descriptive keywords")
    }
}

func TestValidateTitleQuality_AcronymMustBeDefined(t *testing.T) {
    md := `# TLS Deployment Guidance
2025-01-01

Body without defining TLS.

## References
1. A — https://a`
    if err := ValidateTitleQuality(md); err == nil {
        t.Fatalf("expected failure for undefined acronym in title")
    }

    // Now define the acronym in the body
    md2 := `# TLS Deployment Guidance
2025-01-01

Transport Layer Security (TLS) configuration tips.

## References
1. A — https://a`
    if err := ValidateTitleQuality(md2); err != nil {
        t.Fatalf("expected success when acronym is defined, got %v", err)
    }
}


// Tests for FEATURE_CHECKLIST item 273: Heading audit — require descriptive
// mini-title headings, consistent hierarchy/parallel phrasing; optional
// auto-numbering for long reports (numbering tested in app package).
func TestValidateHeadingsQuality_OK(t *testing.T) {
    md := `# T
2025-01-01

## Executive summary
A brief overview of findings and recommendations.

## Background and context
Why this matters and prior art.

### Prior work
Survey of approaches.

### Current constraints
Budget, data availability, and deadlines.

## Risks and limitations
List of constraints.

## References
1. A — https://a`
    if err := ValidateHeadingsQuality(md); err != nil {
        t.Fatalf("expected headings quality to pass, got %v", err)
    }
}

func TestValidateHeadingsQuality_NonDescriptiveFails(t *testing.T) {
    md := `# T
2025-01-01

## Introduction
Text.

## Background
Text.

## Risks and limitations
Text.

## References
1. A — https://a`
    if err := ValidateHeadingsQuality(md); err == nil {
        t.Fatalf("expected headings quality to fail for non-descriptive mini-titles")
    }
}

func TestValidateHeadingsQuality_LevelJumpFails(t *testing.T) {
    md := `# T
2025-01-01

## Section one details
Text.

#### Jumped too far
Text.

## Risks and limitations
Text.

## References
1. A — https://a`
    if err := ValidateHeadingsQuality(md); err == nil {
        t.Fatalf("expected failure for heading level jump (H2 -> H4)")
    }
}

func TestValidateHeadingsQuality_ParallelPhrasingMismatch(t *testing.T) {
    md := `# T
2025-01-01

## Evaluating options
Text.

### Choosing providers
Some.

### Cost analysis
Some.

### Compare latency
Some.

## Risks and limitations
Text.

## References
1. A — https://a`
    // Sibling H3 headings mix gerund starts (Choosing) with non-gerund (Cost, Compare)
    if err := ValidateHeadingsQuality(md); err == nil {
        t.Fatalf("expected failure for parallel phrasing mismatch among sibling headings")
    }
}

