package app

import (
    "strings"
    "testing"

    "github.com/hyperifyio/goresearch/internal/verify"
)

func TestAppendEvidenceAppendix_Success(t *testing.T) {
    base := "# Title\n\nBody.\n\n## References\n1. A — https://a.example\n"
    res := verify.Result{
        Summary: "2 claims extracted; 2 supported; 0 low-confidence.",
        Claims: []verify.Claim{
            {Text: "Claim one [1]", Citations: []int{1}, Confidence: "medium", Supported: true},
            {Text: "Claim two [1]", Citations: []int{1}, Confidence: "medium", Supported: true},
        },
    }
    out := appendEvidenceAppendix(base, res, nil)
    if !strings.Contains(out, "## Evidence check") {
        t.Fatalf("expected evidence section")
    }
    if !strings.Contains(out, res.Summary) {
        t.Fatalf("expected summary in appendix")
    }
    if !strings.Contains(out, "- Claim one [1] — cites [1]; confidence: medium; supported: true") {
        t.Fatalf("expected formatted claim line: got\n%s", out)
    }
}

func TestAppendEvidenceAppendix_GracefulFailure(t *testing.T) {
    base := "# Title\n\nBody.\n\n## References\n1. A — https://a.example\n"
    out := appendEvidenceAppendix(base, verify.Result{}, assertErr{})
    if out != base {
        t.Fatalf("expected unchanged markdown on error")
    }
}

type assertErr struct{}

func (assertErr) Error() string { return "fail" }


