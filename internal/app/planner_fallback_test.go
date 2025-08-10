package app

import (
    "context"
    "testing"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/planner"
)

// Verifies FEATURE_CHECKLIST.md item "Planner failure recovery" — when the LLM
// planner cannot produce parseable output or is unavailable, the system must
// deterministically fall back to generated queries to keep progress.
// See: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestPlanQueries_FallsBackOnPlannerFailure(t *testing.T) {
    a := &App{cfg: Config{LanguageHint: "en"}}
    // Configure facade explicitly: an LLM planner with no client/model will fail,
    // ensuring fallback path is taken.
    a.planner.llm = &planner.LLMPlanner{}
    a.planner.fb = &planner.FallbackPlanner{LanguageHint: a.cfg.LanguageHint}

    b := brief.Brief{Topic: "Cursor MDC format"}
    got := a.planQueries(context.Background(), b)
    if lq := len(got.Queries); lq < 6 || lq > 10 {
        t.Fatalf("expected 6..10 fallback queries, got %d", lq)
    }
    if len(got.Outline) < 5 {
        t.Fatalf("expected fallback outline length >= 5, got %d", len(got.Outline))
    }
    // First few deterministic queries should include the known intent words.
    // Check presence rather than exact positions for stability.
    want1 := b.Topic + " specification (" + a.cfg.LanguageHint + ")"
    want2 := b.Topic + " documentation (" + a.cfg.LanguageHint + ")"
    found1, found2 := false, false
    for _, q := range got.Queries {
        if q == want1 {
            found1 = true
        }
        if q == want2 {
            found2 = true
        }
    }
    if !found1 || !found2 {
        t.Fatalf("expected language-suffixed deterministic queries; found1=%v found2=%v; queries=%v", found1, found2, got.Queries)
    }
}


