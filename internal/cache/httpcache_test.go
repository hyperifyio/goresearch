package cache

import (
    "context"
    "fmt"
    "testing"
    "time"
)

func TestHTTPCache_LRUEnforcement_Count(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    c := &HTTPCache{Dir: dir}
    urls := []string{"https://a.com/1", "https://a.com/2", "https://a.com/3"}
    for i, u := range urls {
        if err := c.Save(context.Background(), u, "text/html", "", "", []byte(fmt.Sprintf("body-%d", i))); err != nil {
            t.Fatalf("save %d: %v", i, err)
        }
        time.Sleep(10 * time.Millisecond)
    }
    // Touch second to make it MRU compared to first
    if _, err := c.LoadBody(context.Background(), urls[1]); err != nil {
        t.Fatalf("touch body: %v", err)
    }
    removed, err := EnforceHTTPCacheLimits(dir, 0, 2)
    if err != nil { t.Fatalf("enforce: %v", err) }
    if removed != 1 { t.Fatalf("expected 1 removed, got %d", removed) }
    // First should be gone
    if _, err := c.LoadBody(context.Background(), urls[0]); err == nil {
        t.Fatalf("expected oldest evicted")
    }
}

func TestHTTPCache_LRUEnforcement_Bytes(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    c := &HTTPCache{Dir: dir}
    // Save two entries with different sizes
    if err := c.Save(context.Background(), "https://b.com/1", "text/html", "", "", []byte("1111111111")); err != nil {
        t.Fatalf("save 1: %v", err)
    }
    time.Sleep(10 * time.Millisecond)
    if err := c.Save(context.Background(), "https://b.com/2", "text/html", "", "", []byte("22")); err != nil {
        t.Fatalf("save 2: %v", err)
    }
    // Set a byte cap that requires evicting the oldest to fit
    // Compute total size roughly: we'll set a very small max to force at least one eviction
    removed, err := EnforceHTTPCacheLimits(dir, 5, 0)
    if err != nil { t.Fatalf("enforce: %v", err) }
    if removed < 1 {
        t.Fatalf("expected at least 1 removal, got %d", removed)
    }
}
