package search

import (
    "context"
    "encoding/json"
    "errors"
    "os"
    "regexp"
    "strings"
)

// FileProvider loads search results from a local JSON file for offline/testing use.
// The JSON file format is an array of objects: {"title": "...", "url": "...", "snippet": "..."}.
type FileProvider struct {
    Path string
    Policy DomainPolicy // optional: filter results by domain
}

func (f *FileProvider) Name() string { return "file" }

func (f *FileProvider) Search(_ context.Context, query string, limit int) ([]Result, error) {
    if strings.TrimSpace(f.Path) == "" {
        return nil, errors.New("file provider path is empty")
    }
    b, err := os.ReadFile(f.Path)
    if err != nil {
        return nil, err
    }
    var raw []Result
    if err := json.Unmarshal(b, &raw); err != nil {
        return nil, err
    }
    q := strings.ToLower(strings.TrimSpace(query))
    out := make([]Result, 0, len(raw))
    for _, r := range raw {
        if r.URL == "" || r.Title == "" {
            continue
        }
        if q == "" || strings.Contains(strings.ToLower(r.Title), q) || strings.Contains(strings.ToLower(r.Snippet), q) || matchesByTokens(q, r.Title+"\n"+r.Snippet) {
            // Apply optional domain policy
            if f.Policy.Denylist != nil || f.Policy.Allowlist != nil {
                if blocked, _ := isDomainBlocked(r.URL, f.Policy.Allowlist, f.Policy.Denylist); blocked {
                    continue
                }
            }
            r.Source = f.Name()
            out = append(out, r)
            if limit > 0 && len(out) >= limit {
                break
            }
        }
    }
    return out, nil
}

// matchesByTokens performs a loose token-based match between the query and the
// candidate text. It returns true when at least two meaningful tokens (length
// >= 3) from the query appear in the text, making the file provider usable for
// longer, natural-language queries in tests and offline runs.
func matchesByTokens(query, text string) bool {
    query = strings.ToLower(query)
    text = strings.ToLower(text)
    // Split on non-letter/digit characters
    splitter := regexp.MustCompile(`[^a-z0-9]+`)
    qTokens := splitter.Split(query, -1)
    if len(qTokens) == 0 {
        return false
    }
    meaningful := 0
    for _, tok := range qTokens {
        if len(tok) < 3 { // skip very short/common tokens
            continue
        }
        if strings.Contains(text, tok) {
            meaningful++
            if meaningful >= 2 {
                return true
            }
        }
    }
    return false
}


