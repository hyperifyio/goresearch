package app

import (
    "strings"
    "testing"
)

func TestAppendReproFooter_AppendsDeterministicFooter(t *testing.T) {
    base := "# Title\n\n2024-01-01\n\n## References\n1. A — https://a.example\n"
    out := appendReproFooter(base, "gpt-4o-mini", "http://localhost:11434/v1", 3, true, true)
    if !strings.Contains(out, "Reproducibility:") {
        t.Fatalf("expected footer marker present; got:\n%s", out)
    }
    if !strings.Contains(out, "model=gpt-4o-mini") {
        t.Fatalf("expected model field")
    }
    if !strings.Contains(out, "llm_base_url=http://localhost:11434/v1") {
        t.Fatalf("expected base URL field")
    }
    if !strings.Contains(out, "sources_used=3") {
        t.Fatalf("expected sources count")
    }
    if !strings.Contains(out, "http_cache=true") || !strings.Contains(out, "llm_cache=true") {
        t.Fatalf("expected cache booleans true")
    }
}


