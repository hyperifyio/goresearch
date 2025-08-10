package llmtools

import (
    "encoding/json"
    "errors"
    "strconv"
    "regexp"
    "strings"

    openai "github.com/sashabaranov/go-openai"
)

// ToolSpec captures a single callable tool/function exposed to the model.
// JSONSchema must be a valid JSON Schema object encoded as raw JSON.
// Name must be a stable, lowercase, snake_case identifier.
// Description should be concise and imperative.
type ToolSpec struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    JSONSchema  json.RawMessage `json:"json_schema"`
}

// ToolCall is a simplified representation of a tool call returned by the model.
// Arguments holds the raw JSON argument object for the call.
type ToolCall struct {
    ID        string          // provider-assigned call id
    Name      string          // function name
    Arguments json.RawMessage // raw JSON arguments
}

// EncodeTools converts ToolSpec entries into OpenAI-compatible tools array.
func EncodeTools(specs []ToolSpec) []openai.Tool {
    out := make([]openai.Tool, 0, len(specs))
    for _, s := range specs {
        out = append(out, openai.Tool{
            Type: "function",
            Function: &openai.FunctionDefinition{
                Name:        s.Name,
                Description: s.Description,
                Parameters:  s.JSONSchema,
            },
        })
    }
    return out
}

// ParseToolCalls extracts function tool calls from a chat completion response.
func ParseToolCalls(resp openai.ChatCompletionResponse) []ToolCall {
    if len(resp.Choices) == 0 {
        return nil
    }
    msg := resp.Choices[0].Message
    if len(msg.ToolCalls) == 0 {
        return nil
    }
    out := make([]ToolCall, 0, len(msg.ToolCalls))
    for _, tc := range msg.ToolCalls {
        if tc.Type != "function" {
            continue
        }
        out = append(out, ToolCall{
            ID:        tc.ID,
            Name:      tc.Function.Name,
            Arguments: json.RawMessage(tc.Function.Arguments),
        })
    }
    return out
}

// Harmony parsing helpers
var (
    fencedFinalRe = regexp.MustCompile("(?s)```\n?final\n(.*?)```|```final\n(.*?)```|```final\r?\n(.*?)```")
    xmlFinalRe    = regexp.MustCompile("(?s)<final>(.*?)</final>")
)

// ParseHarmony extracts a Harmony-style final answer and any tool calls.
// Behavior:
// - If tool_calls are present, return empty final and the parsed calls (model is requesting tools)
// - Else, try to extract content inside ```final fenced block, or <final>...</final>
// - Else, return the whole assistant message content as the final string
func ParseHarmony(resp openai.ChatCompletionResponse) (final string, calls []ToolCall) {
    calls = ParseToolCalls(resp)
    if len(calls) > 0 {
        return "", calls
    }
    if len(resp.Choices) == 0 {
        return "", nil
    }
    msg := resp.Choices[0].Message
    content := msg.Content
    // Try fenced final
    if m := fencedFinalRe.FindStringSubmatch(content); m != nil {
        // Find the first non-empty capture group
        for i := 1; i < len(m); i++ {
            if strings.TrimSpace(m[i]) != "" {
                return strings.TrimSpace(m[i]), nil
            }
        }
    }
    // Try XML-style <final>
    if m := xmlFinalRe.FindStringSubmatch(content); m != nil && len(m) >= 2 {
        return strings.TrimSpace(m[1]), nil
    }
    // Fallback to whole content
    return strings.TrimSpace(content), nil
}

// ContentForLogging returns a safe string to log from a chat completion
// response, enforcing the CoT redaction policy by default.
//
// When allowCOT is false (default), only the Harmony-style final answer is
// returned (if present). If no final marker exists and there are tool calls,
// a short notice is returned. Raw analysis/commentary is not included.
//
// When allowCOT is true, the full assistant message content is returned so that
// callers can debug interleaved tool-calls within CoT when explicitly enabled.
//
// Requirement: FEATURE_CHECKLIST.md — CoT redaction policy
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func ContentForLogging(resp openai.ChatCompletionResponse, allowCOT bool) string {
    if len(resp.Choices) == 0 {
        return ""
    }
    if allowCOT {
        return strings.TrimSpace(resp.Choices[0].Message.Content)
    }
    // If tools are requested, do not surface CoT; indicate presence of tool calls
    calls := ParseToolCalls(resp)
    if len(calls) > 0 {
        return "(tool_calls present; CoT redacted)"
    }
    // Try to extract explicit final markers only. Do not fall back to whole content.
    if len(resp.Choices) > 0 {
        content := resp.Choices[0].Message.Content
        if m := fencedFinalRe.FindStringSubmatch(content); m != nil {
            for i := 1; i < len(m); i++ {
                if strings.TrimSpace(m[i]) != "" {
                    return strings.TrimSpace(m[i])
                }
            }
        }
        if m := xmlFinalRe.FindStringSubmatch(content); m != nil && len(m) >= 2 {
            return strings.TrimSpace(m[1])
        }
    }
    // No final markers; redact entirely
    return "(CoT redacted)"
}

// Minimal JSON Schema validator for a restricted subset of keywords used by our
// tool arg/result contracts. This is not a full JSON Schema implementation; it
// supports only: type (object, array, string, integer, number, boolean),
// properties, required, additionalProperties (boolean), items (single schema),
// and recursively validates nested objects/arrays.
// Returns nil when value conforms to schema; otherwise an error describing the
// first mismatch found.
func validateAgainstSchema(value any, schema json.RawMessage) error {
    if len(schema) == 0 {
        return nil
    }
    var s map[string]any
    if err := json.Unmarshal(schema, &s); err != nil {
        return err
    }
    // helper: get string field
    getString := func(m map[string]any, k string) string {
        if v, ok := m[k]; ok {
            if str, ok := v.(string); ok {
                return str
            }
        }
        return ""
    }
    // helper: normalize number types
    asFloat := func(v any) (float64, bool) {
        switch t := v.(type) {
        case float64:
            return t, true
        case int:
            return float64(t), true
        default:
            return 0, false
        }
    }

    expectedType := getString(s, "type")
    switch expectedType {
    case "object", "":
        // objects are default if type omitted per our internal use
        obj, ok := value.(map[string]any)
        if !ok {
            return errors.New("schema: expected object")
        }
        // required
        if req, ok := s["required"].([]any); ok {
            for _, r := range req {
                if name, ok := r.(string); ok {
                    if _, present := obj[name]; !present {
                        return errors.New("schema: missing required field: " + name)
                    }
                }
            }
        }
        // properties
        var props map[string]any
        if p, ok := s["properties"].(map[string]any); ok {
            props = p
        }
        for k, v := range obj {
            if props != nil {
                if raw, ok := props[k]; ok {
                    // nested schema
                    b, _ := json.Marshal(raw)
                    if err := validateAgainstSchema(v, b); err != nil {
                        return errors.New("schema: property " + k + ": " + err.Error())
                    }
                    continue
                }
            }
            // additionalProperties: default true; if explicitly false, reject unknowns
            if ap, ok := s["additionalProperties"].(bool); ok && !ap {
                return errors.New("schema: additional property not allowed: " + k)
            }
        }
        return nil
    case "array":
        arr, ok := value.([]any)
        if !ok {
            return errors.New("schema: expected array")
        }
        if items, ok := s["items"]; ok {
            b, _ := json.Marshal(items)
            for i, elem := range arr {
                if err := validateAgainstSchema(elem, b); err != nil {
                    return errors.New("schema: items[" + strconv.Itoa(i) + "]: " + err.Error())
                }
            }
        }
        return nil
    case "string":
        if _, ok := value.(string); !ok {
            return errors.New("schema: expected string")
        }
        return nil
    case "integer":
        if f, ok := asFloat(value); !ok || f != float64(int64(f)) {
            return errors.New("schema: expected integer")
        }
        return nil
    case "number":
        if _, ok := asFloat(value); !ok {
            return errors.New("schema: expected number")
        }
        return nil
    case "boolean":
        if _, ok := value.(bool); !ok {
            return errors.New("schema: expected boolean")
        }
        return nil
    default:
        // unsupported types pass through for now
        return nil
    }
}
