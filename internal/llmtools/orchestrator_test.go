package llmtools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
	if !strings.Contains(toolMsg.Content, "unknown tool") {
		t.Fatalf("expected unknown tool error in content: %q", toolMsg.Content)
	}
}
