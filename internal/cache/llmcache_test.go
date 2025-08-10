package cache

import (
    "context"
    "fmt"
    "testing"
    "time"
)

func TestLLMCache_SaveGet(t *testing.T) {
	tmp := t.TempDir()
	c := &LLMCache{Dir: tmp}
	key := KeyFrom("model", "prompt")
	data := []byte(`{"queries":["a"],"outline":["b"]}`)
	if err := c.Save(context.Background(), key, data); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := c.Get(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("get: %v ok=%v", err, ok)
	}
	if string(got) != string(data) {
		t.Fatalf("mismatch")
	}
}

func TestLLMCache_LRUEnforcement(t *testing.T) {
    tmp := t.TempDir()
    c := &LLMCache{Dir: tmp}
    // Create three entries
    keys := []string{KeyFrom("m","p1"), KeyFrom("m","p2"), KeyFrom("m","p3")}
    for i, k := range keys {
        if err := c.Save(context.Background(), k, []byte(fmt.Sprintf("%d", i))); err != nil {
            t.Fatalf("save %d: %v", i, err)
        }
        // Ensure distinct mtimes by sleeping a tiny amount
        time.Sleep(10 * time.Millisecond)
    }
    // Touch p2 to be most recently used
    if _, ok, _ := c.Get(context.Background(), keys[1]); !ok {
        t.Fatal("expected hit")
    }
    // Enforce count=2 should evict oldest (p1) not recently touched
    removed, err := EnforceLLMCacheLimits(tmp, 0, 2)
    if err != nil { t.Fatalf("enforce: %v", err) }
    if removed != 1 { t.Fatalf("expected 1 removed, got %d", removed) }
    if _, ok, _ := c.Get(context.Background(), keys[0]); ok {
        t.Fatal("expected oldest evicted")
    }
}
