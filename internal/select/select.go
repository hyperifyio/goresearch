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
    // PreferredLanguage, when non-empty, biases ranking toward results whose
    // detected language matches. This is a preference only; non-matching
    // languages are not filtered out.
    PreferredLanguage string
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
    // Precompute detected languages once for stability and to avoid recomputation in comparator
    detected := make([]string, len(sorted))
    if strings.TrimSpace(opt.PreferredLanguage) != "" {
        for i, r := range sorted {
            detected[i] = detectLanguage(strings.Join([]string{r.Title, r.Snippet}, " \n "))
        }
    }

    if opt.PreferPrimary || strings.TrimSpace(opt.PreferredLanguage) != "" {
        sort.SliceStable(sorted, func(i, j int) bool {
            // First, apply language preference if requested
            if lang := strings.TrimSpace(opt.PreferredLanguage); lang != "" {
                mi := strings.EqualFold(detected[i], lang)
                mj := strings.EqualFold(detected[j], lang)
                if mi && !mj {
                    return true
                }
                if mj && !mi {
                    return false
                }
            }
            // Then, apply primary host preference if requested
            if opt.PreferPrimary {
                hi := isPrimaryHost(sorted[i].URL)
                hj := isPrimaryHost(sorted[j].URL)
                if hi && !hj {
                    return true
                }
                if hj && !hi {
                    return false
                }
            }
            // Finally, fall back to snippet-length descending
            return len(sorted[i].Snippet) > len(sorted[j].Snippet)
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
        // Avoid crawling behind search result pages per etiquette policy.
        if isSearchResultsPage(u) {
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

// detectLanguage is a lightweight heuristic detector for common languages.
// It returns lowercase ISO-like codes such as "en" or "es" when it can guess,
// otherwise an empty string. This is intentionally simple to keep selection
// deterministic and dependency-free.
func detectLanguage(text string) string {
    s := " " + strings.ToLower(text) + " "
    // Quick checks for Spanish markers (including accented characters)
    if strings.ContainsAny(s, "áéíóúñ") ||
        strings.Contains(s, " el ") || strings.Contains(s, " la ") || strings.Contains(s, " los ") || strings.Contains(s, " las ") ||
        strings.Contains(s, " de ") || strings.Contains(s, " y ") || strings.Contains(s, " en ") || strings.Contains(s, " para ") ||
        strings.Contains(s, " con ") || strings.Contains(s, " una ") || strings.Contains(s, " uno ") || strings.Contains(s, " este ") ||
        strings.Contains(s, " guía ") || strings.Contains(s, " introducción ") || strings.Contains(s, " arquitectura ") {
        return "es"
    }
    // Quick checks for English markers
    if strings.Contains(s, " the ") || strings.Contains(s, " and ") || strings.Contains(s, " of ") || strings.Contains(s, " to ") || strings.Contains(s, " in ") || strings.Contains(s, " guide ") || strings.Contains(s, " introduction ") {
        return "en"
    }
    return ""
}

// isSearchResultsPage heuristically detects URLs that point to search engine
// results pages rather than primary content. We avoid following these to keep
// the crawl polite and focused on content pages.
func isSearchResultsPage(u *url.URL) bool {
    host := strings.ToLower(u.Host)
    path := strings.ToLower(u.EscapedPath())
    q := u.Query()

    // Common engines: Google, Bing, DuckDuckGo, Yahoo, Baidu, Yandex, Startpage, SearxNG
    if strings.Contains(host, "google.") {
        if strings.HasPrefix(path, "/search") || strings.HasPrefix(path, "/url") || q.Has("q") {
            return true
        }
    }
    if strings.Contains(host, "bing.com") {
        if strings.HasPrefix(path, "/search") || q.Has("q") {
            return true
        }
    }
    if strings.Contains(host, "duckduckgo.com") {
        if q.Has("q") {
            return true
        }
    }
    if strings.Contains(host, "search.yahoo.com") {
        if strings.HasPrefix(path, "/search") || q.Has("p") || q.Has("q") {
            return true
        }
    }
    if strings.Contains(host, "baidu.com") {
        if strings.HasPrefix(path, "/s") || q.Has("wd") || q.Has("word") {
            return true
        }
    }
    if strings.Contains(host, "yandex.") {
        if strings.HasPrefix(path, "/search/") || q.Has("text") {
            return true
        }
    }
    if strings.Contains(host, "startpage.com") {
        if q.Has("q") || strings.Contains(path, "/do/search") {
            return true
        }
    }
    if strings.Contains(host, "searx") || strings.Contains(host, "searxng") {
        if q.Has("q") || strings.Contains(path, "/search") {
            return true
        }
    }
    // Fallback: generic patterns
    if strings.Contains(path, "/search") && (q.Has("q") || q.Has("query") || q.Has("text")) {
        return true
    }
    return false
}
