package selecter

import (
	"testing"

	"github.com/hyperifyio/goresearch/internal/search"
)

func TestSelect_PerDomainCap(t *testing.T) {
	in := []search.Result{
		{Title: "a1", URL: "https://a.com/1", Snippet: "x"},
		{Title: "a2", URL: "https://a.com/2", Snippet: "xx"},
		{Title: "a3", URL: "https://a.com/3", Snippet: "xxx"},
		{Title: "b1", URL: "https://b.com/1", Snippet: "xxxx"},
		{Title: "b2", URL: "https://b.com/2", Snippet: "xxxxx"},
	}
	out := Select(in, Options{MaxTotal: 10, PerDomain: 2})
	var countA, countB int
	for _, r := range out {
		if r.URL[:8] == "https://" {
			// fine
		}
		if r.URL[8] == 'a' {
			countA++
		}
		if r.URL[8] == 'b' {
			countB++
		}
	}
	if countA > 2 || countB > 2 {
		t.Fatalf("per-domain cap exceeded: a=%d b=%d", countA, countB)
	}
}

func TestSelect_LowSignalFilteringBySnippetLength(t *testing.T) {
    in := []search.Result{
        {Title: "weak", URL: "https://a.com/1", Snippet: "ok"},
        {Title: "strong", URL: "https://a.com/2", Snippet: "this is a longer snippet with substance"},
        {Title: "spaces only", URL: "https://b.com/1", Snippet: "    \t  "},
    }
    out := Select(in, Options{MaxTotal: 10, PerDomain: 5, MinSnippetChars: 5})
    for _, r := range out {
        if r.Title == "weak" || r.Title == "spaces only" {
            t.Fatalf("expected low-signal results to be filtered out; got %q", r.Title)
        }
    }
    if len(out) != 1 || out[0].Title != "strong" {
        t.Fatalf("expected only the strong result to remain; got %v", out)
    }
}
