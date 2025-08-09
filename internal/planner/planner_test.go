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
	if len(plan.Queries) != 8 {
		t.Fatalf("expected 8 queries, got %d", len(plan.Queries))
	}
	if len(plan.Outline) < 5 {
		t.Fatalf("expected outline length >= 5, got %d", len(plan.Outline))
	}
	if plan.Queries[0] == "" || plan.Outline[0] == "" {
		t.Fatalf("empty entries not expected")
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
}
