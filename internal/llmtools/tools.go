package llmtools

import (
    "encoding/json"

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
