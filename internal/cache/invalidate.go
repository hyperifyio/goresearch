package cache

import (
    "encoding/json"
    "errors"
    "io/fs"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// ClearDir removes the directory and all contents. It recreates the directory
// afterwards to leave a valid empty cache location.
func ClearDir(dir string) error {
    if strings.TrimSpace(dir) == "" {
        return errors.New("empty dir")
    }
    if err := os.RemoveAll(dir); err != nil {
        return err
    }
    return os.MkdirAll(dir, 0o755)
}

// PurgeHTTPCacheByAge removes HTTP cache entries older than maxAge.
// It inspects <key>.meta.json for SavedAt timestamp and deletes both meta and
// corresponding <key>.body when expired.
func PurgeHTTPCacheByAge(dir string, maxAge time.Duration) (int, error) {
    if maxAge <= 0 {
        return 0, nil
    }
    now := time.Now().UTC()
    removed := 0
    // Iterate only meta files to decide expiration
    err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            return nil
        }
        if !strings.HasSuffix(d.Name(), ".meta.json") {
            return nil
        }
        // Read SavedAt
        b, err := os.ReadFile(path)
        if err != nil {
            return nil // skip unreadable
        }
        var e HTTPEntry
        if err := json.Unmarshal(b, &e); err != nil {
            return nil // skip malformed
        }
        if now.Sub(e.SavedAt) <= maxAge {
            return nil
        }
        // Expired: delete meta and body
        removed++
        _ = os.Remove(path)
        base := strings.TrimSuffix(path, ".meta.json")
        _ = os.Remove(base + ".body")
        return nil
    })
    return removed, err
}

// PurgeLLMCacheByAge removes LLM cache entries older than maxAge based on file
// modification time. LLM cache files use the .json extension and are leaf files.
func PurgeLLMCacheByAge(dir string, maxAge time.Duration) (int, error) {
    if maxAge <= 0 {
        return 0, nil
    }
    now := time.Now().UTC()
    removed := 0
    err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            return nil
        }
        name := d.Name()
        // Skip HTTP metadata/body files
        if strings.HasSuffix(name, ".meta.json") || strings.HasSuffix(name, ".body") {
            return nil
        }
        if !strings.HasSuffix(name, ".json") {
            return nil
        }
        info, err := d.Info()
        if err != nil {
            return nil
        }
        if now.Sub(info.ModTime().UTC()) <= maxAge {
            return nil
        }
        removed++
        _ = os.Remove(path)
        return nil
    })
    return removed, err
}


