package llmtools

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "strings"
    "sync"

    "github.com/hyperifyio/goresearch/internal/extract"
    "github.com/hyperifyio/goresearch/internal/fetch"
    "github.com/hyperifyio/goresearch/internal/search"
)

// MinimalDeps bundles dependencies for the minimal tool surface.
type MinimalDeps struct {
    // Search provider (e.g., SearxNG or file-based). Required for web_search.
    SearchProvider search.Provider
    // Fetch client to retrieve URLs with policy enforcement. Required for fetch_url.
    FetchClient    *fetch.Client
    // Extractor to convert HTML into main text. Defaults to HeuristicExtractor when nil.
    Extractor      extract.Extractor
    // EnablePDF controls whether extract_main_text may attempt minimal PDF text extraction.
    EnablePDF      bool
}

// inMemoryExcerptStore stores extracted documents keyed by deterministic ID.
type inMemoryExcerptStore struct {
    mu   sync.RWMutex
    data map[string]extractedDoc
}

type extractedDoc struct {
    ID    string `json:"id"`
    Title string `json:"title"`
    Text  string `json:"text"`
}

func newExcerptStore() *inMemoryExcerptStore {
    return &inMemoryExcerptStore{data: make(map[string]extractedDoc)}
}

func (s *inMemoryExcerptStore) put(doc extractedDoc) {
    s.mu.Lock()
    s.data[doc.ID] = doc
    s.mu.Unlock()
}

func (s *inMemoryExcerptStore) get(id string) (extractedDoc, bool) {
    s.mu.RLock()
    d, ok := s.data[id]
    s.mu.RUnlock()
    return d, ok
}

func sha256Hex(s string) string {
    sum := sha256.Sum256([]byte(s))
    return hex.EncodeToString(sum[:])
}

// NewMinimalRegistry registers the initial minimal tool surface:
// - web_search
// - fetch_url
// - extract_main_text
// - load_cached_excerpt (IDs produced by extract_main_text)
//
// Requirement: FEATURE_CHECKLIST.md — Minimal tool surface
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func NewMinimalRegistry(deps MinimalDeps) (*Registry, error) {
    r := NewRegistry()
    store := newExcerptStore()

    // Default extractor
    extractor := deps.Extractor
    if extractor == nil {
        extractor = extract.HeuristicExtractor{}
    }

    // web_search
    if deps.SearchProvider == nil {
        return nil, fmt.Errorf("NewMinimalRegistry: SearchProvider is nil")
    }
    webSearchSchema := json.RawMessage(`{
        "type":"object",
        "properties":{
            "q":{"type":"string"},
            "limit":{"type":"integer","minimum":1,"maximum":20}
        },
        "required":["q"]
    }`)
    if err := r.Register(ToolDefinition{
        StableName:  "web_search",
        SemVer:      "v1.0.0",
        Description: "Search the public web and return results",
        JSONSchema:  webSearchSchema,
        Capabilities: []string{"search"},
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            var in struct{
                Q     string `json:"q"`
                Limit int    `json:"limit"`
            }
            if err := json.Unmarshal(args, &in); err != nil {
                return nil, fmt.Errorf("invalid args: %w", err)
            }
            q := strings.TrimSpace(in.Q)
            if q == "" {
                return nil, fmt.Errorf("missing q")
            }
            limit := in.Limit
            if limit <= 0 { limit = 10 }
            if limit > 20 { limit = 20 }
            results, err := deps.SearchProvider.Search(ctx, q, limit)
            if err != nil {
                return nil, err
            }
            // Marshal as stable JSON shape
            type outResult struct{ Title, URL, Snippet, Source string }
            out := struct{ Results []outResult `json:"results"` }{Results: make([]outResult, 0, len(results))}
            for _, r := range results {
                out.Results = append(out.Results, outResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet, Source: r.Source})
            }
            return json.Marshal(out)
        },
    }); err != nil { return nil, err }

    // fetch_url
    if deps.FetchClient == nil {
        return nil, fmt.Errorf("NewMinimalRegistry: FetchClient is nil")
    }
    fetchURLSchema := json.RawMessage(`{
        "type":"object",
        "properties":{ "url": {"type":"string","format":"uri"} },
        "required":["url"]
    }`)
    if err := r.Register(ToolDefinition{
        StableName:  "fetch_url",
        SemVer:      "v1.0.0",
        Description: "Fetch a URL with polite headers and return body",
        JSONSchema:  fetchURLSchema,
        Capabilities: []string{"fetch"},
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            var in struct{ URL string `json:"url"` }
            if err := json.Unmarshal(args, &in); err != nil {
                return nil, fmt.Errorf("invalid args: %w", err)
            }
            u := strings.TrimSpace(in.URL)
            if u == "" { return nil, fmt.Errorf("missing url") }
            body, ct, err := deps.FetchClient.Get(ctx, u)
            if err != nil { return nil, err }
            out := struct{
                ContentType string `json:"content_type"`
                Body        string `json:"body"`
            }{ContentType: ct, Body: string(body)}
            return json.Marshal(out)
        },
    }); err != nil { return nil, err }

    // extract_main_text
    extractSchema := json.RawMessage(`{
        "type":"object",
        "properties":{
            "html":{"type":"string"},
            "content_type":{"type":"string"}
        },
        "required":["html"]
    }`)
    if err := r.Register(ToolDefinition{
        StableName:  "extract_main_text",
        SemVer:      "v1.0.0",
        Description: "Extract readable title and text from HTML (or PDF when enabled)",
        JSONSchema:  extractSchema,
        Capabilities: []string{"extract"},
        Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
            var in struct{
                HTML        string `json:"html"`
                ContentType string `json:"content_type"`
            }
            if err := json.Unmarshal(args, &in); err != nil {
                return nil, fmt.Errorf("invalid args: %w", err)
            }
            html := strings.TrimSpace(in.HTML)
            if html == "" {
                return nil, fmt.Errorf("missing html")
            }
            var doc extract.Document
            ct := strings.ToLower(strings.TrimSpace(in.ContentType))
            if deps.EnablePDF && strings.HasPrefix(ct, "application/pdf") {
                doc = extract.FromPDF([]byte(html))
            } else {
                doc = extractor.Extract([]byte(html))
            }
            id := sha256Hex(doc.Title + "\n" + doc.Text)
            stored := extractedDoc{ID: id, Title: strings.TrimSpace(doc.Title), Text: strings.TrimSpace(doc.Text)}
            store.put(stored)
            return json.Marshal(stored)
        },
    }); err != nil { return nil, err }

    // load_cached_excerpt
    loadSchema := json.RawMessage(`{
        "type":"object",
        "properties":{ "id": {"type":"string"} },
        "required":["id"]
    }`)
    if err := r.Register(ToolDefinition{
        StableName:  "load_cached_excerpt",
        SemVer:      "v1.0.0",
        Description: "Load a previously extracted excerpt by ID",
        JSONSchema:  loadSchema,
        Capabilities: []string{"cache","excerpt"},
        Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
            var in struct{ ID string `json:"id"` }
            if err := json.Unmarshal(args, &in); err != nil {
                return nil, fmt.Errorf("invalid args: %w", err)
            }
            id := strings.TrimSpace(in.ID)
            if id == "" { return nil, fmt.Errorf("missing id") }
            if d, ok := store.get(id); ok {
                return json.Marshal(d)
            }
            return nil, fmt.Errorf("not found: %s", id)
        },
    }); err != nil { return nil, err }

    return r, nil
}
