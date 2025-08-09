package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// HTTPEntry captures enough metadata to support conditional revalidation and
// to return content without hitting the network when valid.
type HTTPEntry struct {
	URL          string    `json:"url"`
	ContentType  string    `json:"content_type"`
	ETag         string    `json:"etag"`
	LastModified string    `json:"last_modified"`
	SavedAt      time.Time `json:"saved_at"`
}

// HTTPCache stores responses on disk as <key>.meta.json and <key>.body where
// key is sha256(url). It is a simple, deterministic cache suitable for tests
// and baseline performance. No eviction policy is included.
type HTTPCache struct {
	Dir string
}

func (c *HTTPCache) ensureDir() error {
	if c == nil || c.Dir == "" {
		return errors.New("cache dir not configured")
	}
	return os.MkdirAll(c.Dir, 0o755)
}

func (c *HTTPCache) key(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}

func (c *HTTPCache) metaPath(key string) string { return filepath.Join(c.Dir, key+".meta.json") }
func (c *HTTPCache) bodyPath(key string) string { return filepath.Join(c.Dir, key+".body") }

// LoadMeta returns entry metadata if present.
func (c *HTTPCache) LoadMeta(_ context.Context, url string) (*HTTPEntry, error) {
	if err := c.ensureDir(); err != nil {
		return nil, err
	}
	key := c.key(url)
	f, err := os.Open(c.metaPath(key))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var e HTTPEntry
	if err := json.NewDecoder(f).Decode(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

// LoadBody returns cached body if present.
func (c *HTTPCache) LoadBody(_ context.Context, url string) ([]byte, error) {
	if err := c.ensureDir(); err != nil {
		return nil, err
	}
	key := c.key(url)
	return os.ReadFile(c.bodyPath(key))
}

// Save stores a new cache entry to disk.
func (c *HTTPCache) Save(_ context.Context, url string, contentType string, etag string, lastModified string, body []byte) error {
	if err := c.ensureDir(); err != nil {
		return err
	}
	key := c.key(url)
	// Write body first
	if err := os.WriteFile(c.bodyPath(key), body, 0o644); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	meta := HTTPEntry{
		URL:          url,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: lastModified,
		SavedAt:      time.Now().UTC(),
	}
	tmp := c.metaPath(key) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create meta: %w", err)
	}
	if err := json.NewEncoder(f).Encode(&meta); err != nil {
		f.Close()
		return fmt.Errorf("encode meta: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, c.metaPath(key))
}

// CopyMeta copies meta from an io.Reader (useful for tests)
func (c *HTTPCache) CopyMeta(_ context.Context, url string, r io.Reader) error {
	if err := c.ensureDir(); err != nil {
		return err
	}
	key := c.key(url)
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return os.WriteFile(c.metaPath(key), data, 0o644)
}
