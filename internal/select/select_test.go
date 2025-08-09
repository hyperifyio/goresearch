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
