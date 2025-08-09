package cache

import (
    "context"
    "os"
    "path/filepath"
    "testing"
)

func TestLLMCache_StrictPerms(t *testing.T) {
    t.Parallel()
    base := t.TempDir()
    dir := filepath.Join(base, "llm")
    c := &LLMCache{Dir: dir, StrictPerms: true}
    key := KeyFrom("model", "prompt")
    data := []byte(`{"ok":true}`)
    if err := c.Save(context.Background(), key, data); err != nil {
        t.Fatalf("save: %v", err)
    }
    // Directory should exist with 0700
    info, err := os.Stat(dir)
    if err != nil {
        t.Fatalf("stat dir: %v", err)
    }
    if got := info.Mode() & 0o777; got != 0o700 {
        t.Fatalf("dir mode = %o, want 0700", got)
    }
    // File should be 0600
    p := filepath.Join(dir, key+".json")
    finfo, err := os.Stat(p)
    if err != nil {
        t.Fatalf("stat file: %v", err)
    }
    if got := finfo.Mode() & 0o777; got != 0o600 {
        t.Fatalf("file mode = %o, want 0600", got)
    }
}

func TestHTTPCache_StrictPerms(t *testing.T) {
    t.Parallel()
    base := t.TempDir()
    dir := filepath.Join(base, "http")
    c := &HTTPCache{Dir: dir, StrictPerms: true}
    url := "https://example.com/x"
    body := []byte("hello")
    if err := c.Save(context.Background(), url, "text/html", "etag", "", body); err != nil {
        t.Fatalf("save: %v", err)
    }
    // Directory should exist with 0700
    info, err := os.Stat(dir)
    if err != nil {
        t.Fatalf("stat dir: %v", err)
    }
    if got := info.Mode() & 0o777; got != 0o700 {
        t.Fatalf("dir mode = %o, want 0700", got)
    }
    key := c.key(url)
    // Body and meta files should be 0600
    files := []string{filepath.Join(dir, key+".body"), filepath.Join(dir, key+".meta.json")}
    for _, f := range files {
        finfo, err := os.Stat(f)
        if err != nil {
            t.Fatalf("stat %s: %v", f, err)
        }
        if got := finfo.Mode() & 0o777; got != 0o600 {
            t.Fatalf("%s mode = %o, want 0600", f, got)
        }
    }
}
