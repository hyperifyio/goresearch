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

    "github.com/hyperifyio/goresearch/internal/budget"
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
    // DryRunTools, when true, does not execute tool handlers. Instead it appends
    // a structured tool message that records the intended tool name and a
    // redacted view of the arguments. This is useful for debugging prompt↔tool
    // interplay without performing network or filesystem operations.
    //
    // Requirement: FEATURE_CHECKLIST.md — Dry-run for tools
    // Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
    DryRunTools bool

    // Fallback, when non-nil, is invoked to produce a final answer using the
    // legacy planner→search→synthesis pipeline in cases where tool use is not
    // possible or the model declines to call tools. This satisfies the
    // "Fallback path" checklist item.
    //
    // Trigger conditions:
    // - No tools are registered in the Registry (adapter disabled)
    // - The model response contains no tool_calls (model didn't call tools)
    //
    // The function should return the final Markdown and an error if it fails.
    Fallback func(ctx context.Context) (string, error)
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

    // Seed conversation (system message will be augmented with prompt affordances)
    messages := make([]openai.ChatCompletionMessage, 0, 2+len(extra))
    if system != "" {
        // Build concise prompt affordances describing available tools and error codes
        afford := buildPromptAffordances(o.Registry)
        sys := system
        if afford != "" {
            sys = sys + "\n\n" + afford
        }
        messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: sys})
    }
    messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: user})
    if len(extra) > 0 {
        messages = append(messages, extra...)
    }

    // Encode tool specs from registry. If no tools are available and a fallback
    // is provided, immediately use the fallback pipeline rather than attempting
    // a chat without tools.
    specs := o.Registry.Specs()
    tools := EncodeTools(specs)
    if len(tools) == 0 && o.Fallback != nil {
        final, err := o.Fallback(ctx)
        return final, messages, err
    }

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
        // Apply token/context budgeting: prune or compress earlier turns so the
        // running conversation stays within the model's context window.
        req.Messages = budgetMessagesForRequest(messages, baseReq)
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
            // No tool calls requested. If a fallback pipeline is configured,
            // prefer it over returning free-form assistant content so we
            // preserve the existing planner→search→synthesis contract.
            if o.Fallback != nil {
                fbFinal, err := o.Fallback(ctx)
                return fbFinal, messages, err
            }
            // Otherwise, treat the assistant content as final.
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
            if o.DryRunTools {
                // Build a structured envelope noting dry-run with redacted args
                var argAny any
                _ = json.Unmarshal(call.Arguments, &argAny)
                argAny = scrubValue(argAny)
                env := map[string]any{
                    "ok":       true,
                    "tool":     call.Name,
                    "dry_run":  true,
                    "args":     argAny,
                }
                b, _ := json.Marshal(env)
                resultContent = string(b)
                okFlag = true
            } else if def, ok := o.Registry.Get(call.Name); ok && def.Handler != nil {
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
                // Validate tool arguments against JSON schema before invoking handler.
                // If invalid, surface a structured E_ARGS error without calling the handler.
                var argVal any
                _ = json.Unmarshal(call.Arguments, &argVal)
                if def.JSONSchema != nil && len(def.JSONSchema) > 0 {
                    if vErr := validateAgainstSchema(argVal, def.JSONSchema); vErr != nil {
                        env := map[string]any{
                            "ok":   false,
                            "tool": call.Name,
                            "error": map[string]any{
                                "code":    "E_ARGS",
                                "message": "invalid args: " + scrubString(vErr.Error()),
                            },
                        }
                        b, _ := json.Marshal(env)
                        resultContent = string(b)
                        // Append tool message below as usual
                        messages = append(messages, openai.ChatCompletionMessage{
                            Role:       openai.ChatMessageRoleTool,
                            Name:       call.Name,
                            ToolCallID: call.ID,
                            Content:    resultContent,
                        })
                        toolCallsUsed++
                        // Continue to the next tool call without executing handler
                        continue
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
                Bool("dry_run", o.DryRunTools).
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
        // Apply in-transcript compression of older tool messages to keep the
        // transcript lean over time. Keep the latest 2 tool messages verbatim.
        messages = compressOlderToolMessages(messages, 2)
        // Next iteration uses the augmented messages
    }
}

// buildPromptAffordances renders a short, token-efficient note describing the
// available tools (name, version, and one-line description), basic usage, limits,
// and common error codes that handlers may return. This guides the model to use
// tools correctly without verbose manuals.
//
// Requirement: FEATURE_CHECKLIST.md — Prompt affordances
// Source: https://github.com/hyperifyio/goresearch/blob/main/FEATURE_CHECKLIST.md
func buildPromptAffordances(r *Registry) string {
    if r == nil {
        return ""
    }
    specs := r.Specs()
    if len(specs) == 0 {
        return ""
    }
    // Map spec name -> version and description from registry defs
    // Registry.Specs already includes version in description tail, but we want
    // a stable, concise listing.
    lines := make([]string, 0, 8+len(specs))
    lines = append(lines, "Tools available:")
    // Retrieve full definitions to get SemVer and Description without the suffix.
    for _, s := range specs {
        def, ok := r.Get(s.Name)
        if !ok {
            continue
        }
        // Format: - name (vX.Y.Z): description. Args: JSON object per schema.
        ver := def.SemVer
        if ver == "" {
            ver = "v0.0.0"
        }
        // Keep each tool line short.
        lines = append(lines, fmt.Sprintf("- %s (%s): %s", def.StableName, ver, def.Description))
    }
    // Generic usage and limits
    lines = append(lines,
        "Use tools via tool_calls only. Provide minimal, valid JSON args.",
        "Respect result size budgets: large bodies may return truncated previews and an id to load full content.",
        "Errors are structured: {'ok':false,'error':{'code','message'}}. Common codes: E_ARGS (bad/missing args), E_TIMEOUT (handler timed out), E_POLICY (robots/opt-out/deny), E_NOT_FOUND, E_RESULT_SCHEMA, E_TOOL.")
    return strings.Join(lines, "\n")
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

// budgetMessagesForRequest returns a copy of messages pruned and/or compressed
// to fit into the model's context window with conservative headroom and output
// reservation. Heuristics:
// - Always keep the first system message if present
// - Prefer keeping the most recent turns; start pruning from the oldest
// - Compress older tool messages by truncating large string fields and marking
//   the envelope with {"compressed":true}
// - If still over budget, drop oldest non-system messages until it fits
func budgetMessagesForRequest(messages []openai.ChatCompletionMessage, baseReq openai.ChatCompletionRequest) []openai.ChatCompletionMessage {
    if len(messages) == 0 {
        return nil
    }
    // Work on a copy
    out := append([]openai.ChatCompletionMessage(nil), messages...)

    model := strings.TrimSpace(baseReq.Model)
    if model == "" {
        model = "gpt-4o-mini" // reasonable default for budgeting
    }
    // Reserve output tokens conservatively if MaxTokens is not set
    reserved := baseReq.MaxTokens
    if reserved <= 0 {
        reserved = 1024
    }

    // Helper to estimate total tokens of current message slice
    estimateTotal := func(msgs []openai.ChatCompletionMessage) int {
        total := 0
        for _, m := range msgs {
            total += budget.EstimateTokens(m.Content)
        }
        return total
    }

    // First pass: if it already fits, return as-is
    maxPrompt := budget.RemainingContextWithHeadroom(model, reserved, 0)
    if maxPrompt <= 0 {
        // Degenerate; keep only the last few messages
        if len(out) > 8 {
            return out[len(out)-8:]
        }
        return out
    }

    // Attempt to compress older tool messages (keep the latest 2 as-is)
    // We scan from the beginning to the penultimate tool messages.
    toolCount := 0
    for _, m := range out {
        if m.Role == openai.ChatMessageRoleTool {
            toolCount++
        }
    }
    toolsToKeep := 2
    toCompress := toolCount - toolsToKeep
    if toCompress < 0 { toCompress = 0 }
    if toCompress > 0 {
        for i := 0; i < len(out) && toCompress > 0; i++ {
            if out[i].Role != openai.ChatMessageRoleTool {
                continue
            }
            // Compress content
            out[i].Content = compressToolContent(out[i].Content)
            toCompress--
        }
    }

    // Check fit; if over, prune from the oldest non-system message
    for estimateTotal(out) > maxPrompt {
        if len(out) <= 1 {
            break
        }
        // Preserve the first system message if present.
        if out[0].Role == openai.ChatMessageRoleSystem {
            // Remove out[1]
            out = append(out[:1], out[2:]...)
        } else {
            out = out[1:]
        }
    }
    return out
}

// compressToolContent tries to parse a tool envelope JSON and produce a
// compact representation that preserves keys and IDs while truncating large
// strings. If parsing fails, it returns a safe truncated text.
func compressToolContent(content string) string {
    var anyVal any
    if err := json.Unmarshal([]byte(content), &anyVal); err != nil {
        // Fallback: truncate plain text
        if len(content) > 256 {
            return content[:200] + "… (compressed)"
        }
        return content
    }
    comp := compressJSONNode(anyVal, 256)
    // Mark compressed at top-level if it is an object
    if m, ok := comp.(map[string]any); ok {
        m["compressed"] = true
        comp = m
    }
    b, err := json.Marshal(comp)
    if err != nil {
        if len(content) > 256 { return content[:200] + "… (compressed)" }
        return content
    }
    return string(b)
}

// compressOlderToolMessages compresses the content of older tool messages in
// the transcript, keeping the latest keepLast tool messages unmodified. It
// returns a new slice with compressed content for older tool entries.
func compressOlderToolMessages(messages []openai.ChatCompletionMessage, keepLast int) []openai.ChatCompletionMessage {
    if keepLast < 0 { keepLast = 0 }
    // Find indexes of tool messages
    toolIdx := make([]int, 0, 8)
    for i, m := range messages {
        if m.Role == openai.ChatMessageRoleTool {
            toolIdx = append(toolIdx, i)
        }
    }
    toCompress := len(toolIdx) - keepLast
    if toCompress <= 0 {
        return messages
    }
    out := append([]openai.ChatCompletionMessage(nil), messages...)
    for i := 0; i < toCompress; i++ {
        idx := toolIdx[i]
        out[idx].Content = compressToolContent(out[idx].Content)
    }
    return out
}

// compressJSONNode walks a JSON-like value and truncates large strings.
// maxLen applies per string value. It preserves id fields verbatim.
func compressJSONNode(v any, maxLen int) any {
    switch t := v.(type) {
    case string:
        if len(t) > maxLen {
            // Keep prefix to retain hint of content
            keep := maxLen - 56
            if keep < 0 { keep = maxLen }
            if keep > len(t) { keep = len(t) }
            return t[:keep] + fmt.Sprintf("… (%d chars, truncated)", len(t))
        }
        return t
    case map[string]any:
        out := make(map[string]any, len(t))
        for k, vv := range t {
            if strings.EqualFold(k, "id") {
                out[k] = vv
                continue
            }
            out[k] = compressJSONNode(vv, maxLen)
        }
        return out
    case []any:
        out := make([]any, len(t))
        for i, vv := range t {
            out[i] = compressJSONNode(vv, maxLen)
        }
        return out
    default:
        return v
    }
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
