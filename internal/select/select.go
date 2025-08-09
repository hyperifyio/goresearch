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

	// Simple heuristic: prefer results with longer snippets first to increase signal
	sorted := make([]search.Result, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Snippet) > len(sorted[j].Snippet)
	})

	out := make([]search.Result, 0, opt.MaxTotal)
	for _, r := range sorted {
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
