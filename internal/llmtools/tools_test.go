package llmtools

import (
    "encoding/json"
    "testing"

    openai "github.com/sashabaranov/go-openai"
)

func TestEncodeTools_MapsSpecsToOpenAITools(t *testing.T) {
    schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)
    specs := []ToolSpec{{
        Name:        "web_search",
        Description: "Search the web and return results",
        JSONSchema:  schema,
    }}
    tools := EncodeTools(specs)
    if len(tools) != 1 {
        t.Fatalf("expected 1 tool, got %d", len(tools))
    }
    if tools[0].Type != "function" {
        t.Fatalf("expected tool type 'function', got %q", tools[0].Type)
    }
    if tools[0].Function.Name != "web_search" {
        t.Fatalf("unexpected function name: %q", tools[0].Function.Name)
    }
    if tools[0].Function.Description != "Search the web and return results" {
        t.Fatalf("unexpected function description: %q", tools[0].Function.Description)
    }
    // Parameters is an interface; when set with json.RawMessage it should be []byte
    pm, ok := tools[0].Function.Parameters.(json.RawMessage)
    if !ok {
        t.Fatalf("parameters should be json.RawMessage, got %T", tools[0].Function.Parameters)
    }
    if string(pm) != string(schema) {
        t.Fatalf("parameters mismatch: %s != %s", string(pm), string(schema))
    }
}

func TestParseToolCalls_ReadsAssistantToolCalls(t *testing.T) {
    // Build response JSON to avoid depending on internal Go types for tool_calls
    raw := []byte(`{
        "choices":[{
            "message":{
                "role":"assistant",
                "tool_calls":[{
                    "id":"call_1",
                    "type":"function",
                    "function":{"name":"web_search","arguments":"{\"q\":\"kubernetes\"}"}
                }]
            }
        }]
    }`)
    var resp openai.ChatCompletionResponse
    if err := json.Unmarshal(raw, &resp); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    calls := ParseToolCalls(resp)
    if len(calls) != 1 {
        t.Fatalf("expected 1 tool call, got %d", len(calls))
    }
    if calls[0].Name != "web_search" {
        t.Fatalf("unexpected tool name: %q", calls[0].Name)
    }
    var args map[string]any
    if err := json.Unmarshal(calls[0].Arguments, &args); err != nil {
        t.Fatalf("arguments should be valid JSON: %v", err)
    }
    if got := args["q"]; got != "kubernetes" {
        t.Fatalf("unexpected arg q: %v", got)
    }
}

func TestParseToolCalls_NoChoicesSafe(t *testing.T) {
    var resp openai.ChatCompletionResponse
    calls := ParseToolCalls(resp)
    if len(calls) != 0 {
        t.Fatalf("expected 0 tool calls, got %d", len(calls))
    }
}

func TestValidateAgainstSchema_MinimalSubset(t *testing.T) {
    // object with required and properties
    schema := json.RawMessage(`{
        "type":"object",
        "properties":{
            "a":{"type":"string"},
            "b":{"type":"integer"}
        },
        "required":["a"],
        "additionalProperties": false
    }`)
    val := map[string]any{"a": "x", "b": 3.0}
    if err := validateAgainstSchema(val, schema); err != nil {
        t.Fatalf("unexpected validate error: %v", err)
    }
    // missing required
    val2 := map[string]any{"b": 1.0}
    if err := validateAgainstSchema(val2, schema); err == nil {
        t.Fatalf("expected error for missing required")
    }
    // additional property not allowed
    val3 := map[string]any{"a":"x","c":true}
    if err := validateAgainstSchema(val3, schema); err == nil {
        t.Fatalf("expected error for additional property")
    }
    // array items schema
    arrSchema := json.RawMessage(`{"type":"array","items":{"type":"string"}}`)
    arr := []any{"x","y"}
    if err := validateAgainstSchema(arr, arrSchema); err != nil {
        t.Fatalf("unexpected array validate error: %v", err)
    }
}

// Lightweight fuzz test for validateAgainstSchema to ensure it doesn't panic
// on random JSON values and simple schemas, and returns either nil or error.
func FuzzValidateAgainstSchema_ObjectAndArray(f *testing.F) {
    // seed with a few simple cases
    f.Add(`{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`, `{"a":"x"}`)
    f.Add(`{"type":"array","items":{"type":"integer"}}`, `[1,2,3]`)
    f.Add(`{"type":"string"}`, `"hello"`)
    f.Fuzz(func(t *testing.T, schemaJSON string, valueJSON string) {
        var sch json.RawMessage = json.RawMessage(schemaJSON)
        var val any
        _ = json.Unmarshal([]byte(valueJSON), &val)
        _ = validateAgainstSchema(val, sch) // must not panic
    })
}
