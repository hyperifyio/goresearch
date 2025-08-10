package llmtools

import (
    "bytes"
    "context"
    "encoding/json"
    "regexp"
    "testing"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
    openai "github.com/sashabaranov/go-openai"
)

// Requirement: FEATURE_CHECKLIST.md — Structured tracing
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func TestStructuredTracing_ToolCallLogged(t *testing.T) {
    // Capture logs
    var buf bytes.Buffer
    old := log.Logger
    log.Logger = zerolog.New(&buf).With().Timestamp().Logger()
    t.Cleanup(func() { log.Logger = old })

    // Assistant first requests a tool, then returns final
    rawResp1 := `{
        "choices":[{
            "message":{
                "role":"assistant",
                "tool_calls":[{"id":"tc1","type":"function","function":{"name":"echo","arguments":"{\"msg\":\"hello\"}"}}]
            }
        }]
    }`
    rawResp2 := "{\n\t\"choices\":[{\n\t\t\"message\":{\n\t\t\t\"role\":\"assistant\",\n\t\t\t\"content\":\"<final>ok</final>\"\n\t\t}\n\t}]\n}"

    client := &stubClient{responses: []openai.ChatCompletionResponse{mustUnmarshalResp(t, rawResp1), mustUnmarshalResp(t, rawResp2)}}

    // Minimal registry with a simple tool
    r := NewRegistry()
    _ = r.Register(ToolDefinition{
        StableName:  "echo",
        SemVer:      "v1.0.0",
        Description: "echo message",
        JSONSchema:  jsonObj(map[string]any{"type":"object","properties":map[string]any{"msg":map[string]any{"type":"string"}},"required":[]string{"msg"}}),
        ResultSchema: jsonObj(map[string]any{"type":"object","properties":map[string]any{"msg":map[string]any{"type":"string"}},"required":[]string{"msg"}}),
        Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
            // pass-through
            return args, nil
        },
    })

    orch := &Orchestrator{Client: client, Registry: r}
    if _, _, err := orch.Run(context.Background(), openai.ChatCompletionRequest{Model: "gpt-oss"}, "s", "u", nil); err != nil {
        t.Fatalf("Run error: %v", err)
    }

    logs := buf.String()
    // Expect a structured trace line with required fields
    must := []string{
        `"stage":"tool"`,
        `"tool":"echo"`,
        `"tool_call_id":"tc1"`,
        `"args_hash":"`,
        `"args_bytes":`,
        `"result_bytes":`,
        `"ok":true`,
        `"duration_ms":`,
    }
    for _, needle := range must {
        if !bytes.Contains([]byte(logs), []byte(needle)) {
            t.Fatalf("expected logs to contain %s; got:\n%s", needle, logs)
        }
    }
    // args_hash should look like lowercase hex
    if !regexp.MustCompile(`"args_hash":"[0-9a-f]{64}"`).MatchString(logs) {
        t.Fatalf("args_hash hex not found in logs: %s", logs)
    }
}
