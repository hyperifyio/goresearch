package app

import (
    "context"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/hyperifyio/goresearch/internal/search"
)

// Test fetchAndExtract chooses PDF extractor when enabled and content-type is application/pdf.
func TestFetchAndExtract_PDFSwitch(t *testing.T) {
    pdfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/pdf")
        _, _ = w.Write([]byte("%PDF-1.4\nstream\n(Hello PDF App) Tj\nendstream\n"))
    }))
    defer pdfSrv.Close()

    // sourceGetter that returns Content-Type from server
    getter := sourceGetterFunc(func(ctx context.Context, url string) ([]byte, string, error) {
        resp, err := http.Get(url)
        if err != nil {
            return nil, "", err
        }
        defer resp.Body.Close()
        b, _ := io.ReadAll(resp.Body)
        return b, resp.Header.Get("Content-Type"), nil
    })

    selected := []search.Result{{Title: "PDF", URL: pdfSrv.URL}}
    cfg := Config{PerSourceChars: 1000, EnablePDF: true}
    excerpts, skipped := fetchAndExtract(context.Background(), getter, nil, selected, cfg)
    if len(excerpts) != 1 || len(skipped) != 0 {
        t.Fatalf("expected 1 excerpt and 0 skipped, got excerpts=%d skipped=%d", len(excerpts), len(skipped))
    }
    if !strings.Contains(excerpts[0].Excerpt, "Hello PDF App") {
        t.Fatalf("expected pdf text extraction, got: %q", excerpts[0].Excerpt)
    }
}

// sourceGetterFunc adapts a function to sourceGetter interface for tests.
type sourceGetterFunc func(ctx context.Context, url string) ([]byte, string, error)

func (f sourceGetterFunc) get(ctx context.Context, url string) ([]byte, string, error) { return f(ctx, url) }
