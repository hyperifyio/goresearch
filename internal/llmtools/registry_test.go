package llmtools

import (
    "context"
    "encoding/json"
    "errors"
    "testing"
)

func mustRaw(t *testing.T, v any) json.RawMessage {
    t.Helper()
    b, err := json.Marshal(v)
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    return b
}

func TestRegistry_RegisterAndSpecsAndCatalog(t *testing.T) {
    r := NewRegistry()

    // Valid definition
    def := ToolDefinition{
        StableName:  "web_search",
        SemVer:      "v1.0.0",
        Description: "search the web for results",
        JSONSchema: mustRaw(t, map[string]any{
            "type": "object",
            "properties": map[string]any{
                "q": map[string]any{"type": "string"},
                "limit": map[string]any{"type": "integer", "minimum": 1},
            },
            "required": []string{"q"},
        }),
        Capabilities: []string{"search", "query"},
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            // Echo back query for testing
            var a struct {
                Q     string `json:"q"`
                Limit int    `json:"limit"`
            }
            _ = json.Unmarshal(args, &a)
            return mustRaw(t, map[string]any{"echo": a.Q, "limit": a.Limit}), nil
        },
    }

    if err := r.Register(def); err != nil {
        t.Fatalf("Register: %v", err)
    }

    // Deterministic Specs and EncodeTools
    specs := r.Specs()
    if len(specs) != 1 {
        t.Fatalf("expected 1 spec, got %d", len(specs))
    }
    if specs[0].Name != "web_search" {
        t.Fatalf("unexpected spec name: %s", specs[0].Name)
    }
    if specs[0].Description == "" || specs[0].Description == def.Description {
        t.Fatalf("expected description to include version suffix, got: %q", specs[0].Description)
    }

    tools := EncodeTools(specs)
    if tools[0].Function == nil || tools[0].Function.Name != "web_search" {
        t.Fatalf("EncodeTools: wrong function mapping")
    }

    // Catalog contains stable name, version, and capabilities
    meta := r.Catalog()
    if len(meta) != 1 {
        t.Fatalf("expected 1 meta entry, got %d", len(meta))
    }
    if meta[0].StableName != "web_search" || meta[0].SemVer != "v1.0.0" {
        t.Fatalf("unexpected meta: %+v", meta[0])
    }
    if len(meta[0].Capabilities) != 2 {
        t.Fatalf("capabilities not preserved: %+v", meta[0].Capabilities)
    }

    // Get and invoke handler
    gotDef, ok := r.Get("web_search")
    if !ok {
        t.Fatalf("Get did not find tool")
    }
    res, err := gotDef.Handler(context.Background(), mustRaw(t, map[string]any{"q": "golang", "limit": 3}))
    if err != nil {
        t.Fatalf("handler error: %v", err)
    }
    var out map[string]any
    _ = json.Unmarshal(res, &out)
    if out["echo"] != "golang" {
        t.Fatalf("unexpected handler output: %v", out)
    }
}

func TestRegistry_RegisterValidation(t *testing.T) {
    r := NewRegistry()

    // Invalid name
    err := r.Register(ToolDefinition{
        StableName:  "Invalid-Name",
        SemVer:      "v0.1.0",
        Description: "x",
        JSONSchema:  mustRaw(t, map[string]any{"type": "object"}),
        Handler:     func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil },
    })
    if err == nil {
        t.Fatalf("expected error for invalid name")
    }

    // Invalid semver
    err = r.Register(ToolDefinition{
        StableName:  "fetch_url",
        SemVer:      "1.0", // invalid
        Description: "x",
        JSONSchema:  mustRaw(t, map[string]any{"type": "object"}),
        Handler:     func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil },
    })
    if err == nil {
        t.Fatalf("expected error for invalid semver")
    }

    // Non-object schema
    err = r.Register(ToolDefinition{
        StableName:  "extract_main_text",
        SemVer:      "v0.1.0",
        Description: "x",
        JSONSchema:  mustRaw(t, []any{"not", "an", "object"}),
        Handler:     func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil },
    })
    if err == nil {
        t.Fatalf("expected error for non-object schema")
    }

    // Nil handler
    err = r.Register(ToolDefinition{
        StableName:  "load_cached_excerpt",
        SemVer:      "v0.1.0",
        Description: "x",
        JSONSchema:  mustRaw(t, map[string]any{"type": "object"}),
        Handler:     nil,
    })
    if err == nil {
        t.Fatalf("expected error for nil handler")
    }
}

func TestRegistry_DeterministicOrdering(t *testing.T) {
    r := NewRegistry()

    defs := []ToolDefinition{
        {StableName: "fetch_url", SemVer: "v1.0.0", Description: "fetch", JSONSchema: mustRaw(t, map[string]any{"type": "object"}), Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil }},
        {StableName: "web_search", SemVer: "v1.0.0", Description: "search", JSONSchema: mustRaw(t, map[string]any{"type": "object"}), Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil }},
        {StableName: "extract_main_text", SemVer: "v1.0.0", Description: "extract", JSONSchema: mustRaw(t, map[string]any{"type": "object"}), Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil }},
    }
    for _, d := range defs {
        if err := r.Register(d); err != nil {
            t.Fatalf("register %s: %v", d.StableName, err)
        }
    }

    specs := r.Specs()
    if got, want := specs[0].Name, "extract_main_text"; got != want {
        t.Fatalf("unexpected order: got %s want %s", got, want)
    }
    if got, want := specs[1].Name, "fetch_url"; got != want {
        t.Fatalf("unexpected order: got %s want %s", got, want)
    }
    if got, want := specs[2].Name, "web_search"; got != want {
        t.Fatalf("unexpected order: got %s want %s", got, want)
    }
}

func TestRegistry_HandlerErrorPropagation(t *testing.T) {
    r := NewRegistry()
    r.Register(ToolDefinition{
        StableName:  "failing_tool",
        SemVer:      "v0.0.1",
        Description: "always fails",
        JSONSchema:  mustRaw(t, map[string]any{"type": "object"}),
        Handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
            return nil, errors.New("bad args: missing q")
        },
    })

    def, ok := r.Get("failing_tool")
    if !ok {
        t.Fatalf("tool not found")
    }
    if _, err := def.Handler(context.Background(), mustRaw(t, map[string]any{})); err == nil {
        t.Fatalf("expected handler error")
    }
}
