package llmtools

import (
    "context"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "regexp"
    "net/url"
    "strings"
    "time"

    "github.com/rs/zerolog/log"
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
            startedTool := time.Now()
            argsHashBytes := sha256.Sum256([]byte(call.Arguments))
            argsHash := fmt.Sprintf("%x", argsHashBytes[:])
            argsBytes := len(call.Arguments)
            okFlag := false
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
                    // Map error to typed envelope
                    code := classifyToolError(err)
                    env := map[string]any{
                        "ok":   false,
                        "tool": call.Name,
                        "error": map[string]any{
                            "code":    code,
                            "message": scrubString(err.Error()),
                        },
                    }
                    b, _ := json.Marshal(env)
                    resultContent = string(b)
                } else {
                    // Validate successful result against tool ResultSchema when provided
                    var val any
                    if len(raw) > 0 {
                        _ = json.Unmarshal(raw, &val)
                    }
                    // Safety redaction: scrub secrets, cookies, tracking params from data
                    val = scrubValue(val)
                    var vErr error
                    if def.ResultSchema != nil && len(def.ResultSchema) > 0 {
                        vErr = validateAgainstSchema(val, def.ResultSchema)
                    }
                    if vErr != nil {
                        env := map[string]any{
                            "ok":   false,
                            "tool": call.Name,
                            "error": map[string]any{
                                "code":    "E_RESULT_SCHEMA",
                                "message": "tool result failed schema validation: " + vErr.Error(),
                            },
                        }
                        b, _ := json.Marshal(env)
                        resultContent = string(b)
                    } else {
                        // Wrap successful result in a typed envelope for consistency
                        env := map[string]any{
                            "ok":   true,
                            "tool": call.Name,
                            "data": val,
                        }
                        b, _ := json.Marshal(env)
                        resultContent = string(b)
                        okFlag = true
                    }
                }
            } else {
                // Unknown tool
                env := map[string]any{
                    "ok":   false,
                    "tool": call.Name,
                    "error": map[string]any{
                        "code":    "E_UNKNOWN_TOOL",
                        "message": "unknown tool",
                    },
                }
                b, _ := json.Marshal(env)
                resultContent = string(b)
            }

            // Structured tracing per tool call
            log.Info().
                Str("stage", "tool").
                Str("tool", call.Name).
                Str("tool_call_id", call.ID).
                Str("args_hash", argsHash).
                Int("args_bytes", argsBytes).
                Int("result_bytes", len(resultContent)).
                Bool("ok", okFlag).
                Int64("duration_ms", time.Since(startedTool).Milliseconds()).
                Msg("tool call")

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

// classifyToolError maps common error strings to stable error codes so models
// can implement retries or corrective actions deterministically.
func classifyToolError(err error) string {
    if err == nil {
        return ""
    }
    msg := err.Error()
    switch {
    case strings.Contains(msg, "invalid args"):
        return "E_ARGS"
    case strings.Contains(msg, "missing "):
        return "E_ARGS"
    case strings.Contains(msg, "not found"):
        return "E_NOT_FOUND"
    case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
        return "E_TIMEOUT"
    case strings.Contains(msg, "forbidden") || strings.Contains(msg, "disallow"):
        return "E_POLICY"
    default:
        return "E_TOOL"
    }
}

// scrubValue walks arbitrary JSON-like data and scrubs sensitive information.
// - Strings that look like URLs are normalized (drop fragments, lowercase host),
//   tracking params are removed, and sensitive param values (token, key, secret, password)
//   are replaced with "[redacted]"; userinfo in URLs is removed.
// - Header-like strings such as Authorization/Cookie/Set-Cookie have values redacted.
// - Maps and arrays are processed recursively.
func scrubValue(v any) any {
    switch t := v.(type) {
    case string:
        return scrubString(t)
    case map[string]any:
        out := make(map[string]any, len(t))
        for k, vv := range t {
            out[k] = scrubValue(vv)
        }
        return out
    case []any:
        out := make([]any, len(t))
        for i, vv := range t {
            out[i] = scrubValue(vv)
        }
        return out
    default:
        return v
    }
}

var (
    reAuthHeader   = regexp.MustCompile(`(?i)(authorization\s*:\s*)([^\r\n]+)`) // Authorization: ....
    reCookieHeader = regexp.MustCompile(`(?i)\b(set-cookie|cookie)\s*:\s*[^\r\n]+`) // full header
    reBearer       = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-+/=]+`)
)

// scrubString redacts header-like secrets and sanitizes URL strings.
func scrubString(s string) string {
    // Redact common header leaks
    s = reAuthHeader.ReplaceAllString(s, "$1[redacted]")
    s = reCookieHeader.ReplaceAllString(s, "$1: [redacted]")
    s = reBearer.ReplaceAllString(s, "Bearer [redacted]")

    // Try to parse as a full URL and sanitize
    if u, err := urlParseMaybe(s); err == nil && u.Scheme != "" && u.Host != "" {
        return sanitizeURLForSafety(u)
    }
    return s
}

// urlParseMaybe parses a string as URL if it appears to be a URL.
func urlParseMaybe(s string) (*url.URL, error) {
    return url.Parse(s)
}

// sanitizeURLForSafety applies sanitizeURLString plus secret param redaction and userinfo removal.
func sanitizeURLForSafety(u *url.URL) string {
    // remove userinfo
    u.User = nil
    // drop fragment and lowercase host; remove tracking params
    cleaned := sanitizeURLString(u.String())
    uu, err := url.Parse(cleaned)
    if err != nil { return cleaned }
    // Redact sensitive param values
    q := uu.Query()
    for _, key := range []string{"token", "access_token", "id_token", "api_key", "apikey", "x_api_key", "key", "secret", "password", "auth"} {
        if _, ok := q[key]; ok {
            q.Del(key) // drop existing to control order
            q.Add(key, "[redacted]")
        }
    }
    uu.RawQuery = q.Encode()
    return uu.String()
}
