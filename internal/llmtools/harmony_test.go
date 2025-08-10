package llmtools

import (
    "encoding/json"
    "testing"

    openai "github.com/sashabaranov/go-openai"
)

// The Harmony helper should extract only the final answer when present,
// ignore analysis/commentary text, and surface tool calls when provided.

func TestParseHarmony_FinalFenceBlock(t *testing.T) {
    resp := openai.ChatCompletionResponse{}
    resp.Choices = []openai.ChatCompletionChoice{{
        Message: openai.ChatCompletionMessage{
            Role:    openai.ChatMessageRoleAssistant,
            Content: "thinking...\n```final\nRESULT\n```\n",
        },
    }}
    gotFinal, gotCalls := ParseHarmony(resp)
    if gotFinal != "RESULT" {
        t.Fatalf("final mismatch: %q", gotFinal)
    }
    if len(gotCalls) != 0 {
        t.Fatalf("expected no tool calls, got %d", len(gotCalls))
    }
}

func TestParseHarmony_XMLStyleFinalTag(t *testing.T) {
    resp := openai.ChatCompletionResponse{}
    resp.Choices = []openai.ChatCompletionChoice{{
        Message: openai.ChatCompletionMessage{
            Role:    openai.ChatMessageRoleAssistant,
            Content: "<analysis>blah</analysis>\n<final>Answer here</final>",
        },
    }}
    gotFinal, _ := ParseHarmony(resp)
    if gotFinal != "Answer here" {
        t.Fatalf("final mismatch: %q", gotFinal)
    }
}

func TestParseHarmony_DefaultsToWholeContentWhenNoMarkers(t *testing.T) {
    resp := openai.ChatCompletionResponse{}
    resp.Choices = []openai.ChatCompletionChoice{{
        Message: openai.ChatCompletionMessage{
            Role:    openai.ChatMessageRoleAssistant,
            Content: "# ok",
        },
    }}
    gotFinal, _ := ParseHarmony(resp)
    if gotFinal != "# ok" {
        t.Fatalf("final mismatch: %q", gotFinal)
    }
}

func TestParseHarmony_PrefersToolCallsOverContent(t *testing.T) {
    // Build response JSON with tool_calls present; content should be ignored for final.
    raw := []byte(`{
        "choices":[{
            "message":{
                "role":"assistant",
                "content":"analysis text that should be ignored",
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
    gotFinal, calls := ParseHarmony(resp)
    if gotFinal != "" {
        t.Fatalf("expected empty final when tool_calls present, got %q", gotFinal)
    }
    if len(calls) != 1 || calls[0].Name != "web_search" {
        t.Fatalf("unexpected tool calls: %+v", calls)
    }
}
