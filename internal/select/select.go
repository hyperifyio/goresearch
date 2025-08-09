package selecter

import (
    "net/url"
    "sort"
    "strings"

    "github.com/hyperifyio/goresearch/internal/search"
)

// Options configures selection constraints.
type Options struct {
	MaxTotal      int
	PerDomain     int
	PreferPrimary bool
    // MinSnippetChars drops results whose snippet has fewer than this many
    // non-whitespace characters. Zero disables low-signal filtering.
    MinSnippetChars int
}

// Select applies diversity-aware selection with per-domain caps.
func Select(results []search.Result, opt Options) []search.Result {
	if opt.MaxTotal <= 0 {
		opt.MaxTotal = 10
	}
	if opt.PerDomain <= 0 {
		opt.PerDomain = 3
	}
	// Normalize by URL host and dedupe by canonical URL string
	domainCounts := map[string]int{}
	seenURL := map[string]struct{}{}

    // Simple heuristic: prefer results with longer snippets first to increase signal.
    // If PreferPrimary is set, apply a stable reordering that bumps known
    // authoritative hosts (e.g., standards bodies and vendor docs) to the top.
    sorted := make([]search.Result, len(results))
    copy(sorted, results)
    if opt.PreferPrimary {
        sort.SliceStable(sorted, func(i, j int) bool {
            li, lj := len(sorted[i].Snippet), len(sorted[j].Snippet)
            hi := isPrimaryHost(sorted[i].URL)
            hj := isPrimaryHost(sorted[j].URL)
            if hi && !hj {
                return true
            }
            if hj && !hi {
                return false
            }
            return li > lj
        })
    } else {
        sort.SliceStable(sorted, func(i, j int) bool {
            return len(sorted[i].Snippet) > len(sorted[j].Snippet)
        })
    }

    out := make([]search.Result, 0, opt.MaxTotal)
	for _, r := range sorted {
        if opt.MinSnippetChars > 0 {
            // Treat very short snippets as low-signal and skip them early.
            if len(strings.TrimSpace(r.Snippet)) < opt.MinSnippetChars {
                continue
            }
        }
		u, err := url.Parse(strings.TrimSpace(r.URL))
		if err != nil || u.Host == "" {
			continue
		}
		canon := canonicalizeURL(u)
		if _, ok := seenURL[canon]; ok {
			continue
		}
		host := strings.ToLower(u.Host)
		if domainCounts[host] >= opt.PerDomain {
			continue
		}
		seenURL[canon] = struct{}{}
		domainCounts[host]++
		out = append(out, r)
		if len(out) >= opt.MaxTotal {
			break
		}
	}
	return out
}

func canonicalizeURL(u *url.URL) string {
	// drop fragments and default ports; lower-case host
	u2 := *u
	u2.Fragment = ""
	u2.Host = strings.ToLower(u2.Host)
	if (u2.Scheme == "http" && strings.HasSuffix(u2.Host, ":80")) || (u2.Scheme == "https" && strings.HasSuffix(u2.Host, ":443")) {
		host := u2.Hostname()
		u2.Host = host
	}
	return u2.String()
}

// isPrimaryHost returns true if the URL host appears to be an authoritative
// source for technical specifications or primary vendor documentation.
func isPrimaryHost(rawURL string) bool {
    u, err := url.Parse(strings.TrimSpace(rawURL))
    if err != nil || u.Host == "" {
        return false
    }
    h := strings.ToLower(u.Host)
    if h == "developer.mozilla.org" || h == "whatwg.org" || h == "www.w3.org" {
        return true
    }
    // Heuristic for vendor primary docs (e.g., docs.python.org, docs.microsoft.com)
    if strings.HasPrefix(h, "docs.") || strings.HasSuffix(h, ".apache.org") {
        return true
    }
    return false
}
