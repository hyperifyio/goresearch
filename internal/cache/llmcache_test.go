package cache

import (
	"context"
	"testing"
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
