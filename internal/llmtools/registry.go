package llmtools

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "regexp"
    "sort"
    "strings"
)

// ToolHandler executes a tool using the provided raw JSON arguments and returns
// a raw JSON result or an error. The handler must be deterministic for the same
// inputs and side-effect free beyond allowed I/O defined by the tool contract.
//
// Errors must be actionable and safe to surface back into a transcript.
type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// ToolDefinition describes a callable tool with stable identity and metadata.
// StableName must be lowercase snake_case and never change across versions.
// SemVer follows semantic versioning (allowing a leading 'v').
// Capabilities list high-level behaviors for audit/reproducibility reports.
type ToolDefinition struct {
    StableName   string          // stable, lowercase snake_case identifier
    SemVer       string          // semantic version (e.g., v1.2.3)
    Description  string          // concise, imperative description
    JSONSchema   json.RawMessage // JSON Schema for arguments
    Capabilities []string        // capability tags (e.g., "search", "fetch")
    Handler      ToolHandler     // function implementing the tool
}

// ToolMeta is a minimal, serializable view for manifests and logs.
type ToolMeta struct {
    StableName   string   `json:"stable_name"`
    SemVer       string   `json:"semver"`
    Capabilities []string `json:"capabilities"`
}

// Registry holds the set of available tools keyed by stable name.
// Names are unique; updating a tool should bump SemVer.
type Registry struct {
    nameToDef map[string]ToolDefinition
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
    return &Registry{nameToDef: make(map[string]ToolDefinition)}
}

var (
    nameRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
    semverRe = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

// Register adds or replaces a tool definition by stable name after validation.
// It validates stable name, semver, and that the schema is a JSON object.
func (r *Registry) Register(def ToolDefinition) error {
    if def.StableName == "" || !nameRe.MatchString(def.StableName) {
        return fmt.Errorf("invalid stable name %q: must be lowercase snake_case starting with a letter", def.StableName)
    }
    if def.SemVer == "" || !semverRe.MatchString(def.SemVer) {
        return fmt.Errorf("invalid semver %q: must follow semantic versioning", def.SemVer)
    }
    if len(def.JSONSchema) == 0 || !isJSONObject(def.JSONSchema) {
        return errors.New("json schema must be a non-empty JSON object")
    }
    if def.Handler == nil {
        return errors.New("handler must not be nil")
    }
    // Defensive copy of capabilities without empty/whitespace-only entries.
    cleanedCaps := make([]string, 0, len(def.Capabilities))
    for _, c := range def.Capabilities {
        c = strings.TrimSpace(c)
        if c != "" {
            cleanedCaps = append(cleanedCaps, c)
        }
    }
    def.Capabilities = cleanedCaps
    if r.nameToDef == nil {
        r.nameToDef = make(map[string]ToolDefinition)
    }
    r.nameToDef[def.StableName] = def
    return nil
}

// Specs returns OpenAI-compatible tool specs derived from the registered tools.
// The order is deterministic (sorted by stable name) for reproducibility.
func (r *Registry) Specs() []ToolSpec {
    names := make([]string, 0, len(r.nameToDef))
    for name := range r.nameToDef {
        names = append(names, name)
    }
    sort.Strings(names)
    specs := make([]ToolSpec, 0, len(names))
    for _, name := range names {
        def := r.nameToDef[name]
        // Include version hint in description tail to aid humans; name remains stable.
        description := def.Description
        if def.SemVer != "" {
            description = fmt.Sprintf("%s (version %s)", description, def.SemVer)
        }
        specs = append(specs, ToolSpec{
            Name:        def.StableName,
            Description: description,
            JSONSchema:  def.JSONSchema,
        })
    }
    return specs
}

// Get returns a tool definition by stable name if present.
func (r *Registry) Get(stableName string) (ToolDefinition, bool) {
    def, ok := r.nameToDef[stableName]
    return def, ok
}

// Catalog returns a deterministic, sorted slice of ToolMeta for reproducibility.
func (r *Registry) Catalog() []ToolMeta {
    names := make([]string, 0, len(r.nameToDef))
    for name := range r.nameToDef {
        names = append(names, name)
    }
    sort.Strings(names)
    out := make([]ToolMeta, 0, len(names))
    for _, name := range names {
        def := r.nameToDef[name]
        out = append(out, ToolMeta{
            StableName:   def.StableName,
            SemVer:       def.SemVer,
            Capabilities: append([]string(nil), def.Capabilities...),
        })
    }
    return out
}

// isJSONObject returns true if the raw JSON represents a JSON object.
func isJSONObject(raw json.RawMessage) bool {
    var any interface{}
    if err := json.Unmarshal(raw, &any); err != nil {
        return false
    }
    _, ok := any.(map[string]interface{})
    return ok
}
