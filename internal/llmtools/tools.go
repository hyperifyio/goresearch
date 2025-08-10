package llmtools

import (
    "encoding/json"
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
