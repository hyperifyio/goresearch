package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// EnforceHTTPCacheLimits enforces maxBytes and/or maxCount for HTTP cache entries.
// When limits are exceeded, it evicts the least-recently-used entries first based
// on the newest mtime among the meta/body pair. A non-positive limit disables
// that dimension. Returns the number of entries removed.
func EnforceHTTPCacheLimits(dir string, maxBytes int64, maxCount int) (int, error) {
    if strings.TrimSpace(dir) == "" {
        return 0, errors.New("empty dir")
    }
    if maxBytes <= 0 && maxCount <= 0 {
        return 0, nil
    }
    type entry struct{
        base   string // full path without extension suffix
        mtime  time.Time
        bytes  int64 // meta + body size
    }
    entries := make([]entry, 0, 64)
    var totalBytes int64
    var totalCount int
    // Build list from meta files; treat missing body as size 0 but still evictable
    err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        if d.IsDir() { return nil }
        name := d.Name()
        if !strings.HasSuffix(name, ".meta.json") { return nil }
        base := strings.TrimSuffix(path, ".meta.json")
        // Stat meta and body
        var size int64
        var mt time.Time
        if info, err := os.Stat(path); err == nil {
            size += info.Size()
            mt = info.ModTime().UTC()
        }
        if info, err := os.Stat(base+".body"); err == nil {
            size += info.Size()
            bmt := info.ModTime().UTC()
            if bmt.After(mt) { mt = bmt }
        }
        entries = append(entries, entry{base: base, mtime: mt, bytes: size})
        totalBytes += size
        totalCount++
        return nil
    })
    if err != nil { return 0, err }

    // Sort by oldest mtime first (LRU)
    sort.Slice(entries, func(i, j int) bool { return entries[i].mtime.Before(entries[j].mtime) })

    removed := 0
    // Helper to check if over limits
    over := func() bool {
        if maxCount > 0 && totalCount > maxCount { return true }
        if maxBytes > 0 && totalBytes > maxBytes { return true }
        return false
    }
    idx := 0
    for over() && idx < len(entries) {
        e := entries[idx]
        // Remove meta and body; ignore individual errors
        _ = os.Remove(e.base + ".meta.json")
        _ = os.Remove(e.base + ".body")
        totalBytes -= e.bytes
        totalCount--
        removed++
        idx++
    }
    return removed, nil
}

// EnforceLLMCacheLimits enforces maxBytes and/or maxCount for LLM cache entries.
// Entries are single .json files that are not HTTP .meta.json. Eviction order is
// least-recently-used by file mtime.
func EnforceLLMCacheLimits(dir string, maxBytes int64, maxCount int) (int, error) {
    if strings.TrimSpace(dir) == "" {
        return 0, errors.New("empty dir")
    }
    if maxBytes <= 0 && maxCount <= 0 {
        return 0, nil
    }
    type entry struct{
        path  string
        mtime time.Time
        bytes int64
    }
    entries := make([]entry, 0, 64)
    var totalBytes int64
    var totalCount int
    err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        if d.IsDir() { return nil }
        name := d.Name()
        if !strings.HasSuffix(name, ".json") { return nil }
        if strings.HasSuffix(name, ".meta.json") { return nil } // HTTP meta
        info, err := d.Info()
        if err != nil { return nil }
        entries = append(entries, entry{path: path, mtime: info.ModTime().UTC(), bytes: info.Size()})
        totalBytes += info.Size()
        totalCount++
        return nil
    })
    if err != nil { return 0, err }

    sort.Slice(entries, func(i, j int) bool { return entries[i].mtime.Before(entries[j].mtime) })
    removed := 0
    over := func() bool {
        if maxCount > 0 && totalCount > maxCount { return true }
        if maxBytes > 0 && totalBytes > maxBytes { return true }
        return false
    }
    idx := 0
    for over() && idx < len(entries) {
        e := entries[idx]
        if err := os.Remove(e.path); err != nil {
            // If we cannot remove, stop to avoid spinning
            return removed, fmt.Errorf("remove %s: %w", e.path, err)
        }
        totalBytes -= e.bytes
        totalCount--
        removed++
        idx++
    }
    return removed, nil
}
