package aggregate

import (
	"net/url"
	"strings"

	"github.com/hyperifyio/goresearch/internal/search"
)

// MergeAndNormalize merges results from multiple queries, canonicalizes URLs,
// trims obvious tracking parameters, and de-duplicates exact URLs.
func MergeAndNormalize(groups [][]search.Result) []search.Result {
	seen := map[string]struct{}{}
	out := make([]search.Result, 0, 64)
	for _, g := range groups {
		for _, r := range g {
			if r.URL == "" {
				continue
			}
			u, err := url.Parse(r.URL)
			if err != nil {
				continue
			}
			normalizeURL(u)
			key := u.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			r.URL = key
			out = append(out, r)
		}
	}
	return out
}

func normalizeURL(u *url.URL) {
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	q := u.Query()
	// Remove common tracking params
	for _, p := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "gclid", "fbclid"} {
		q.Del(p)
	}
	u.RawQuery = q.Encode()
}
