package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
    "time"
)

// LLMCache stores model responses keyed by a normalized prompt digest and model name.
type LLMCache struct {
    Dir         string
    // StrictPerms, when true, enforces 0700 on cache directories and 0600 on
    // files to provide at-rest protection via restricted permissions.
    StrictPerms bool
}

func (c *LLMCache) ensureDir() error {
	if c == nil || c.Dir == "" {
		return errors.New("cache dir not configured")
	}
    perm := os.FileMode(0o755)
    if c.StrictPerms {
        perm = 0o700
    }
    if err := os.MkdirAll(c.Dir, perm); err != nil {
        return err
    }
    // If directory already existed and StrictPerms is on, tighten perms
    if c.StrictPerms {
        if info, err := os.Stat(c.Dir); err == nil {
            if info.Mode()&0o777 != 0o700 {
                _ = os.Chmod(c.Dir, 0o700)
            }
        }
    }
    return nil
}

// KeyFrom builds a cache key from model and prompt digest.
func KeyFrom(model string, prompt string) string {
	h := sha256.Sum256([]byte(model + "\n\n" + prompt))
	return hex.EncodeToString(h[:])
}

func (c *LLMCache) pathFor(key string) string {
	return filepath.Join(c.Dir, key+".json")
}

// Get returns cached bytes if present.
func (c *LLMCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	if err := c.ensureDir(); err != nil {
		return nil, false, err
	}
	p := c.pathFor(key)
    b, err := os.ReadFile(p)
    if err != nil {
        return nil, false, nil
    }
    // Touch file mtime on access for LRU purposes
    now := time.Now()
    _ = os.Chtimes(p, now, now)
	return b, true, nil
}

// Save writes bytes to cache.
func (c *LLMCache) Save(_ context.Context, key string, data []byte) error {
	if err := c.ensureDir(); err != nil {
		return err
	}
	p := c.pathFor(key)
    mode := os.FileMode(0o644)
    if c.StrictPerms {
        mode = 0o600
    }
    return os.WriteFile(p, data, mode)
}
