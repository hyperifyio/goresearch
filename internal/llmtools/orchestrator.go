package llmtools

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    openai "github.com/sashabaranov/go-openai"
)

// ChatClient abstracts the OpenAI client dependency for testability.
// It mirrors the minimal method we use across the codebase.
type ChatClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// Orchestrator coordinates a tool-enabled chat loop until a final answer.
// It sends tool specs, executes any returned tool calls via the registry,
// appends tool results as role=tool messages, and stops on final assistant text.
//
// Requirement: FEATURE_CHECKLIST.md — Orchestration loop
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
//
// Note: Loop guards (max steps, budgets) are implemented in later items.
// This struct focuses on correctness of the tool loop semantics.
type Orchestrator struct {
	Client   ChatClient
	Registry *Registry
    // MaxToolCalls limits the total number of tool calls executed during a single Run.
    // If zero or negative, a default of 32 is used.
    MaxToolCalls int
    // MaxWallClock bounds the total wall-clock duration for the orchestration loop.
    // If zero, no extra deadline is applied beyond the context passed to Run.
    MaxWallClock time.Duration
    // PerToolTimeout bounds the duration of a single tool handler execution.
    // If zero or negative, a default of 10 seconds is used.
    PerToolTimeout time.Duration
}

// Run executes the orchestration loop.
// - baseReq supplies Model and other request settings; Messages, Tools are managed here
// - system and user are turned into the first two messages in the conversation
// - extra can include any additional messages to seed the chat (optional)
// Returns the final assistant text (Harmony-parsed) and the full transcript used.
func (o *Orchestrator) Run(ctx context.Context, baseReq openai.ChatCompletionRequest, system, user string, extra []openai.ChatCompletionMessage) (string, []openai.ChatCompletionMessage, error) {
    if o.Client == nil {
        return "", nil, fmt.Errorf("orchestrator: Client is nil")
    }
    if o.Registry == nil {
        return "", nil, fmt.Errorf("orchestrator: Registry is nil")
    }

    // Seed conversation
    messages := make([]openai.ChatCompletionMessage, 0, 2+len(extra))
    if system != "" {
        messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: system})
    }
    messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: user})
    if len(extra) > 0 {
        messages = append(messages, extra...)
    }

    // Encode tool specs from registry
    specs := o.Registry.Specs()
    tools := EncodeTools(specs)

    // Loop guards
    started := time.Now()
    var deadline time.Time
    if o.MaxWallClock > 0 {
        deadline = started.Add(o.MaxWallClock)
    }
    maxCalls := o.MaxToolCalls
    if maxCalls <= 0 {
        maxCalls = 32
    }
    toolCallsUsed := 0

    // Loop until final content is produced
    for {
        if !deadline.IsZero() && time.Now().After(deadline) {
            return "", messages, fmt.Errorf("orchestrator: wall-clock budget exceeded")
        }

        req := baseReq
        req.Messages = append([]openai.ChatCompletionMessage(nil), messages...)
        req.Tools = tools

        // Respect remaining wall-clock time for the request if set
        reqCtx := ctx
        var cancelReq context.CancelFunc
        if !deadline.IsZero() {
            remain := time.Until(deadline)
            if remain <= 0 {
                return "", messages, fmt.Errorf("orchestrator: wall-clock budget exceeded")
            }
            reqCtx, cancelReq = context.WithTimeout(ctx, remain)
        }
        resp, err := o.Client.CreateChatCompletion(reqCtx, req)
        if cancelReq != nil {
            cancelReq()
        }
        if err != nil {
            return "", messages, err
        }

        // Append assistant response to transcript
        if len(resp.Choices) == 0 {
            return "", messages, fmt.Errorf("orchestrator: empty choices from model")
        }
        assistantMsg := resp.Choices[0].Message
        messages = append(messages, assistantMsg)

        final, calls := ParseHarmony(resp)
        if len(calls) == 0 {
            // No tool calls requested; final content (if any) ends the loop
            return final, messages, nil
        }

        // Guard: max total tool calls per run
        if toolCallsUsed+len(calls) > maxCalls {
            return "", messages, fmt.Errorf("orchestrator: max tool calls exceeded: used=%d, pending=%d, max=%d", toolCallsUsed, len(calls), maxCalls)
        }

        // Execute tool calls in order, append tool results, then continue loop
        for _, call := range calls {
            resultContent := ""
            if def, ok := o.Registry.Get(call.Name); ok && def.Handler != nil {
                // Compute effective timeout respecting remaining wall-clock and per-tool timeout
                per := o.PerToolTimeout
                if per <= 0 {
                    per = 10 * time.Second
                }
                eff := per
                if !deadline.IsZero() {
                    remain := time.Until(deadline)
                    if remain <= 0 {
                        return "", messages, fmt.Errorf("orchestrator: wall-clock budget exceeded")
                    }
                    if remain < eff {
                        eff = remain
                    }
                }
                toolCtx, cancelTool := context.WithTimeout(ctx, eff)
                raw, err := def.Handler(toolCtx, call.Arguments)
                cancelTool()
                if err != nil {
                    // Surface structured error object as JSON in tool content
                    obj := map[string]any{"error": err.Error(), "tool": call.Name}
                    b, _ := json.Marshal(obj)
                    resultContent = string(b)
                } else {
                    // Ensure tool content is JSON-encoded string (even if empty)
                    if len(raw) == 0 {
                        resultContent = "{}"
                    } else {
                        resultContent = string(raw)
                    }
                }
            } else {
                // Unknown tool: return structured error
                obj := map[string]any{"error": "unknown tool", "tool": call.Name}
                b, _ := json.Marshal(obj)
                resultContent = string(b)
            }

            messages = append(messages, openai.ChatCompletionMessage{
                Role:       openai.ChatMessageRoleTool,
                Name:       call.Name,
                ToolCallID: call.ID,
                Content:    resultContent,
            })
            toolCallsUsed++
        }
        // Next iteration uses the augmented messages
    }
}
