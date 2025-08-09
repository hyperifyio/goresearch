package verify

import (
    "strings"
    "testing"
)

func TestFallbackVerifyExtractsClaimsAndConfidence(t *testing.T) {
    md := "# Title\n\nThis report studies X. It finds that the protocol was standardized in 2018 [1] and updated in 2020 [2]. Further, independent evaluations confirm performance gains of 30% [3][4]. Some statements lack direct evidence here.\n\n## References\n1. Spec — https://example.com/spec\n2. Update — https://example.com/update\n3. Study — https://example.com/study\n4. Benchmark — https://example.com/bench\n"
    got := fallbackVerify(md)
    if len(got.Claims) == 0 {
        t.Fatalf("expected some claims, got none")
    }
    // Expect at least one claim with two citations -> high confidence
    foundHigh := false
    for _, c := range got.Claims {
        if c.Confidence == "high" {
            foundHigh = true
            break
        }
    }
    if !foundHigh {
        t.Fatalf("expected at least one high-confidence claim")
    }
}

// Verifies go-implement item: "Verification test cases" — ensure unsupported claims are flagged.
func TestFallbackVerifyFlagsUnsupportedClaims(t *testing.T) {
    md := "# Title\n\nThis is a cited statement that references evidence [1]. However, this next sentence deliberately contains no citations and should be flagged as unsupported because it lacks brackets and sources.\n\n## References\n1. Ref — https://example.com\n"
    got := fallbackVerify(md)
    if len(got.Claims) == 0 {
        t.Fatalf("expected some claims, got none")
    }
    foundUnsupported := false
    for _, c := range got.Claims {
        if !c.Supported {
            foundUnsupported = true
            if c.Confidence != "low" {
                t.Fatalf("unsupported claim must be low confidence, got %q", c.Confidence)
            }
            break
        }
    }
    if !foundUnsupported {
        t.Fatalf("expected at least one unsupported claim (no citations)")
    }
    if !strings.Contains(got.Summary, "low-confidence") {
        t.Fatalf("expected summary to mention low-confidence count, got %q", got.Summary)
    }
}


