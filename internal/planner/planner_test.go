package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/hyperifyio/goresearch/internal/brief"
)

func TestFallbackPlanner_Deterministic(t *testing.T) {
	b := brief.Brief{Topic: "Cursor MDC format", TargetLengthWords: 1200}
	p := &FallbackPlanner{LanguageHint: "en"}
	plan, err := p.Plan(context.Background(), b)
	if err != nil {
		t.Fatalf("fallback plan error: %v", err)
	}
    if lq := len(plan.Queries); lq < 6 || lq > 10 {
        t.Fatalf("expected 6..10 queries, got %d", lq)
    }
    if len(plan.Outline) < 6 {
        t.Fatalf("expected outline length >= 6, got %d", len(plan.Outline))
    }
	if plan.Queries[0] == "" || plan.Outline[0] == "" {
		t.Fatalf("empty entries not expected")
	}
    // Must include counter-evidence queries
    wantSubs := []string{"limitations", "contrary findings", "alternatives"}
    have := 0
    for _, q := range plan.Queries {
        qq := strings.ToLower(q)
        for _, w := range wantSubs {
            if strings.Contains(qq, w) { have++; break }
        }
    }
    if have < 2 {
        t.Fatalf("expected at least two counter-evidence/alternatives queries, found %d in %v", have, plan.Queries)
    }
}

func TestFallbackPlanner_LanguageHintAppends(t *testing.T) {
	b := brief.Brief{Topic: "Kubernetes"}
	p := &FallbackPlanner{LanguageHint: "es"}
	plan, err := p.Plan(context.Background(), b)
	if err != nil {
		t.Fatalf("fallback plan error: %v", err)
	}
	for _, q := range plan.Queries {
		if !strings.Contains(q, "(es)") {
			t.Fatalf("expected language hint '(es)' appended to query %q", q)
		}
	}
    // Outline must include the Alternatives & conflicting evidence section
    found := false
    for _, h := range plan.Outline {
        if strings.EqualFold(strings.TrimSpace(h), "Alternatives & conflicting evidence") { found = true; break }
    }
    if !found {
        t.Fatalf("expected outline to include 'Alternatives & conflicting evidence', got %v", plan.Outline)
    }
}
