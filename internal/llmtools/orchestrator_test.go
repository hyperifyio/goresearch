package llmtools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
    "time"

	openai "github.com/sashabaranov/go-openai"
)

type stubClient struct{ responses []openai.ChatCompletionResponse }

func (s *stubClient) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	if len(s.responses) == 0 {
		return openai.ChatCompletionResponse{}, context.Canceled // signal no more responses
	}
	r := s.responses[0]
	s.responses = s.responses[1:]
	return r, nil
}

func jsonObj(obj map[string]any) json.RawMessage {
	b, _ := json.Marshal(obj)
	return b
}

func mustUnmarshalResp(t *testing.T, raw string) openai.ChatCompletionResponse {
	t.Helper()
	var resp openai.ChatCompletionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

func TestOrchestrator_ToolLoopAndFinal(t *testing.T) {
	// Response 1: assistant requests a tool call web_search with args {"q":"go"}
	rawResp1 := `{
		"choices":[{
			"message":{
				"role":"assistant",
				"content":"analysis",
				"tool_calls":[{
					"id":"call1",
					"type":"function",
					"function":{"name":"web_search","arguments":"{\"q\":\"go\"}"}
				}]
			}
		}]
	}`
	resp1 := mustUnmarshalResp(t, rawResp1)

	// Response 2: assistant returns final answer with Harmony XML-style final tag
	rawResp2 := "{\n\t\"choices\":[{\n\t\t\"message\":{\n\t\t\t\"role\":\"assistant\",\n\t\t\t\"content\":\"<final>Answer here</final>\"\n\t\t}\n\t}]\n}"
	resp2 := mustUnmarshalResp(t, rawResp2)

	client := &stubClient{responses: []openai.ChatCompletionResponse{resp1, resp2}}

	r := NewRegistry()
	_ = r.Register(ToolDefinition{
		StableName:  "web_search",
		SemVer:      "v1.0.0",
		Description: "Search web",
		JSONSchema:  jsonObj(map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}),
		Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			return jsonObj(map[string]any{"results": []map[string]any{{"title": "Go", "url": "https://go.dev"}}}), nil
		},
	})

	orch := &Orchestrator{Client: client, Registry: r}
	final, transcript, err := orch.Run(context.Background(), openai.ChatCompletionRequest{Model: "gpt-oss"}, "system", "user question", nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if final != "Answer here" {
		t.Fatalf("unexpected final: %q", final)
	}
	// Expect transcript to contain: system, user, assistant(tool call), tool, assistant(final)
	roles := make([]string, 0, len(transcript))
	for _, m := range transcript {
		roles = append(roles, m.Role)
	}
	wantOrder := []string{openai.ChatMessageRoleSystem, openai.ChatMessageRoleUser, openai.ChatMessageRoleAssistant, openai.ChatMessageRoleTool, openai.ChatMessageRoleAssistant}
	if len(roles) != len(wantOrder) {
		t.Fatalf("unexpected transcript length: %v", roles)
	}
	for i := range wantOrder {
		if roles[i] != wantOrder[i] {
			t.Fatalf("role[%d]=%s want %s", i, roles[i], wantOrder[i])
		}
	}
	// Validate tool message fields
	toolMsg := transcript[3]
    if toolMsg.Name != "web_search" || toolMsg.ToolCallID != "call1" || toolMsg.Content == "" {
		t.Fatalf("unexpected tool message: %+v", toolMsg)
	}
    // Tool content is an envelope JSON with ok/tool/data
    var env map[string]any
    if err := json.Unmarshal([]byte(toolMsg.Content), &env); err != nil {
        t.Fatalf("tool content not JSON: %v", err)
    }
    if ok, _ := env["ok"].(bool); !ok {
        t.Fatalf("expected ok=true in tool envelope: %v", env)
    }
}

func TestOrchestrator_UnknownToolProducesStructuredError(t *testing.T) {
	// First assistant turn requests an unknown tool; second returns final
	rawResp1 := `{
		"choices":[{
			"message":{
				"role":"assistant",
				"tool_calls":[{"id":"t1","type":"function","function":{"name":"does_not_exist","arguments":"{}"}}]
			}
		}]
	}`
	rawResp2 := "{\n\t\"choices\":[{\n\t\t\"message\":{\n\t\t\t\"role\":\"assistant\",\n\t\t\t\"content\":\"<final>Done</final>\"\n\t\t}\n\t}]\n}"
	client := &stubClient{responses: []openai.ChatCompletionResponse{mustUnmarshalResp(t, rawResp1), mustUnmarshalResp(t, rawResp2)}}
	r := NewRegistry()
	orch := &Orchestrator{Client: client, Registry: r}
	final, transcript, err := orch.Run(context.Background(), openai.ChatCompletionRequest{Model: "gpt-oss"}, "s", "u", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "Done" {
		t.Fatalf("unexpected final: %q", final)
	}
	// Tool message should include a structured error JSON content
	if len(transcript) < 4 {
		t.Fatalf("short transcript")
	}
	toolMsg := transcript[3]
	if toolMsg.Role != openai.ChatMessageRoleTool {
		t.Fatalf("expected tool role, got %s", toolMsg.Role)
	}
    var env2 map[string]any
    if err := json.Unmarshal([]byte(toolMsg.Content), &env2); err != nil {
        t.Fatalf("tool content not JSON: %v", err)
    }
    if ok, _ := env2["ok"].(bool); ok {
        t.Fatalf("expected ok=false for unknown tool: %v", env2)
    }
    errObj, _ := env2["error"].(map[string]any)
    if errObj["code"] != "E_UNKNOWN_TOOL" {
        t.Fatalf("expected E_UNKNOWN_TOOL, got %v", errObj)
    }
}

func TestOrchestrator_MaxToolCallsExceeded(t *testing.T) {
    // First assistant turn requests a tool; second also requests a tool.
    // With MaxToolCalls=1, the second turn should trigger an error before executing.
    rawResp1 := `{
        "choices":[{
            "message":{
                "role":"assistant",
                "tool_calls":[{"id":"t1","type":"function","function":{"name":"noop","arguments":"{}"}}]
            }
        }]
    }`
    rawResp2 := `{
        "choices":[{
            "message":{
                "role":"assistant",
                "tool_calls":[{"id":"t2","type":"function","function":{"name":"noop","arguments":"{}"}}]
            }
        }]
    }`
    client := &stubClient{responses: []openai.ChatCompletionResponse{mustUnmarshalResp(t, rawResp1), mustUnmarshalResp(t, rawResp2)}}
    r := NewRegistry()
    _ = r.Register(ToolDefinition{
        StableName:  "noop",
        SemVer:      "v1.0.0",
        Description: "no operation",
        JSONSchema:  jsonObj(map[string]any{"type": "object"}),
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            return jsonObj(map[string]any{"ok": true}), nil
        },
    })
    orch := &Orchestrator{Client: client, Registry: r, MaxToolCalls: 1}
    final, transcript, err := orch.Run(context.Background(), openai.ChatCompletionRequest{Model: "gpt-oss"}, "s", "u", nil)
    if err == nil || !strings.Contains(err.Error(), "max tool calls exceeded") {
        t.Fatalf("expected max tool calls exceeded error, got final=%q err=%v", final, err)
    }
    if len(transcript) < 3 { // system, user, assistant(tool call)
        t.Fatalf("unexpected transcript length: %d", len(transcript))
    }
}

func TestOrchestrator_PerToolTimeoutEnforced(t *testing.T) {
    // Assistant requests one tool, then final. Tool handler blocks until ctx timeout.
    rawResp1 := `{
        "choices":[{
            "message":{
                "role":"assistant",
                "tool_calls":[{"id":"t1","type":"function","function":{"name":"block","arguments":"{}"}}]
            }
        }]
    }`
    rawResp2 := "{\n\t\"choices\":[{\n\t\t\"message\":{\n\t\t\t\"role\":\"assistant\",\n\t\t\t\"content\":\"<final>Done</final>\"\n\t\t}\n\t}]\n}"
    client := &stubClient{responses: []openai.ChatCompletionResponse{mustUnmarshalResp(t, rawResp1), mustUnmarshalResp(t, rawResp2)}}
    r := NewRegistry()
    _ = r.Register(ToolDefinition{
        StableName:  "block",
        SemVer:      "v1.0.0",
        Description: "block until ctx done",
        JSONSchema:  jsonObj(map[string]any{"type": "object"}),
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            <-ctx.Done()
            return nil, ctx.Err()
        },
    })
    orch := &Orchestrator{Client: client, Registry: r, PerToolTimeout: 10 * time.Millisecond}
    final, transcript, err := orch.Run(context.Background(), openai.ChatCompletionRequest{Model: "gpt-oss"}, "s", "u", nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if final != "Done" {
        t.Fatalf("unexpected final: %q", final)
    }
    // Tool message content should contain a structured error envelope
    found := false
    for _, m := range transcript {
        if m.Role == openai.ChatMessageRoleTool && m.Name == "block" {
            var env map[string]any
            if err := json.Unmarshal([]byte(m.Content), &env); err != nil { t.Fatalf("tool content not JSON: %v", err) }
            if ok, _ := env["ok"].(bool); ok { t.Fatalf("expected ok=false for timeout: %v", env) }
            errObj, _ := env["error"].(map[string]any)
            if errObj["code"] != "E_TIMEOUT" { t.Fatalf("expected E_TIMEOUT, got %v", errObj) }
            found = true
        }
    }
    if !found {
        t.Fatalf("expected tool message not found")
    }
}

func TestOrchestrator_WallClockBudgetExceeded(t *testing.T) {
    // Single tool call that blocks for a while; set a tiny wall-clock budget so the
    // next model turn should fail with wall-clock exceeded.
    rawResp1 := `{
        "choices":[{
            "message":{
                "role":"assistant",
                "tool_calls":[{"id":"t1","type":"function","function":{"name":"block","arguments":"{}"}}]
            }
        }]
    }`
    // Second response would be final, but we should error before reaching it.
    rawResp2 := "{\n\t\"choices\":[{\n\t\t\"message\":{\n\t\t\t\"role\":\"assistant\",\n\t\t\t\"content\":\"<final>Too late</final>\"\n\t\t}\n\t}]\n}"
    client := &stubClient{responses: []openai.ChatCompletionResponse{mustUnmarshalResp(t, rawResp1), mustUnmarshalResp(t, rawResp2)}}
    r := NewRegistry()
    _ = r.Register(ToolDefinition{
        StableName:  "block",
        SemVer:      "v1.0.0",
        Description: "block until ctx done",
        JSONSchema:  jsonObj(map[string]any{"type": "object"}),
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            <-ctx.Done()
            return nil, ctx.Err()
        },
    })
    orch := &Orchestrator{Client: client, Registry: r, MaxWallClock: 15 * time.Millisecond, PerToolTimeout: 20 * time.Millisecond}
    final, _, err := orch.Run(context.Background(), openai.ChatCompletionRequest{Model: "gpt-oss"}, "s", "u", nil)
    if err == nil || !strings.Contains(err.Error(), "wall-clock budget exceeded") {
        t.Fatalf("expected wall-clock budget error, got final=%q err=%v", final, err)
    }
}
