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

func TestSelect_PreferPrimarySources(t *testing.T) {
    in := []search.Result{
        {Title: "Dev blog long guide", URL: "https://devblog.example.com/html-guide", Snippet: "this is a very very long snippet that would normally dominate purely by length"},
        {Title: "HTML Living Standard — WHATWG", URL: "https://whatwg.org/specs/html/", Snippet: "short"},
        {Title: "MDN Web Docs — HTML", URL: "https://developer.mozilla.org/en-US/docs/Web/HTML", Snippet: "short"},
    }

    // Without PreferPrimary, the longest snippet should sort first
    out := Select(in, Options{MaxTotal: 3, PerDomain: 5, PreferPrimary: false})
    if len(out) == 0 {
        t.Fatalf("expected some results")
    }
    if out[0].Title != "Dev blog long guide" {
        t.Fatalf("expected longest-snippet blog to be first when PreferPrimary is false; got %q", out[0].Title)
    }

    // With PreferPrimary, authoritative sources should be ranked ahead of the blog
    out2 := Select(in, Options{MaxTotal: 3, PerDomain: 5, PreferPrimary: true})
    if len(out2) == 0 {
        t.Fatalf("expected some results")
    }
    topHost := func(u string) string {
        if len(u) < 9 || u[:8] != "https://" {
            return ""
        }
        // crude host extraction for the test; production code parses URL properly
        rest := u[8:]
        for i := 0; i < len(rest); i++ {
            if rest[i] == '/' {
                return rest[:i]
            }
        }
        return rest
    }(out2[0].URL)
    if topHost != "whatwg.org" && topHost != "developer.mozilla.org" {
        t.Fatalf("expected a primary source to be ranked first when PreferPrimary is true; got host %q and title %q", topHost, out2[0].Title)
    }
}

func TestSelect_PreferLanguageMatchWithoutFiltering(t *testing.T) {
    // Spanish vs English; prefer Spanish when PreferredLanguage is "es"
    in := []search.Result{
        {Title: "Intro a Kubernetes", URL: "https://es.example.com/k8s", Snippet: "Esta es una guía introductoria de Kubernetes y su arquitectura."},
        {Title: "Kubernetes overview", URL: "https://en.example.com/k8s", Snippet: "This is an introductory guide to Kubernetes and its architecture."},
    }

    out := Select(in, Options{MaxTotal: 10, PerDomain: 5, PreferPrimary: false, MinSnippetChars: 5, PreferredLanguage: "es"})
    if len(out) != 2 {
        t.Fatalf("expected both results to remain; got %d", len(out))
    }
    if out[0].Title != "Intro a Kubernetes" {
        t.Fatalf("expected Spanish result to be ranked first when PreferredLanguage=es; got %q", out[0].Title)
    }
}
