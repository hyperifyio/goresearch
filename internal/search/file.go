package search

import (
    "context"
    "encoding/json"
    "errors"
    "os"
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
        if q == "" || strings.Contains(strings.ToLower(r.Title), q) || strings.Contains(strings.ToLower(r.Snippet), q) {
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


