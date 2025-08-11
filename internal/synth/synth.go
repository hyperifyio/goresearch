package synth

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"
    "strings"

    openai "github.com/sashabaranov/go-openai"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/cache"
    "github.com/hyperifyio/goresearch/internal/llm"
    "github.com/hyperifyio/goresearch/internal/llmtools"
    "github.com/hyperifyio/goresearch/internal/template"
)

// Uses llm.Client provider interface for backend independence.

// SourceExcerpt contains a single source and its excerpt text to feed the model.
type SourceExcerpt struct {
    Index   int
    Title   string
    URL     string
    Excerpt string
}

// Input bundles all information needed to synthesize the report.
type Input struct {
    Brief         brief.Brief
    Outline       []string
    Sources       []SourceExcerpt
    Model         string
    LanguageHint  string
    ReservedOutputTokens int
}

// Synthesizer calls the LLM to produce a Markdown report per strict contract.
type Synthesizer struct {
    Client llm.Client
    Cache  *cache.LLMCache
    Verbose bool
    // SystemPrompt, when non-empty, overrides the default system message.
    SystemPrompt string
    // AllowCOTLogging enables logging of raw assistant content (CoT) for
    // debugging Harmony/tool-call interplay. Default is false and CoT is redacted.
    AllowCOTLogging bool
    // CacheOnly, when true, returns from cache and fails fast if missing.
    CacheOnly bool
}

// ErrNoSubstantiveBody indicates the model produced no usable Markdown body.
var ErrNoSubstantiveBody = errors.New("no substantive body")

// Synthesize requests a single, cohesive Markdown document following the
// structure and citation rules. It returns the raw Markdown string.
func (s *Synthesizer) Synthesize(ctx context.Context, in Input) (string, error) {
    if s.Client == nil || strings.TrimSpace(in.Model) == "" {
        return "", errors.New("synthesizer not configured")
    }
    system := buildSystemMessage(in.Brief)
    if strings.TrimSpace(s.SystemPrompt) != "" {
        system = s.SystemPrompt
    }
    user := buildUserMessage(in)

    // Cache by model+prompt to allow deterministic re-runs.
    if s.Cache != nil {
        key := cache.KeyFrom(in.Model, system+"\n\n"+user)
        if raw, ok, _ := s.Cache.Get(ctx, key); ok {
            var out struct{ Markdown string `json:"markdown"` }
            if err := json.Unmarshal(raw, &out); err == nil && strings.TrimSpace(out.Markdown) != "" {
                return out.Markdown, nil
            }
        }
    }
    if s.CacheOnly {
        return "", ErrNoSubstantiveBody
    }

    req := openai.ChatCompletionRequest{
        Model: in.Model,
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleSystem, Content: system},
            {Role: openai.ChatMessageRoleUser, Content: user},
        },
        Temperature: 0.1,
        N:           1,
    }
    // Transient-error retry: one short backoff attempt before failing.
    resp, err := s.Client.CreateChatCompletion(ctx, req)
    if err != nil {
        // single retry after short sleep
        // use a tiny, fixed backoff to keep deterministic behavior in tests
        // and avoid long waits in CLI runs
        // Note: context deadline will still bound this.
        // go: no timer imports at top yet
        // implement inline minimal backoff using a channel-based timer
        if sleeper := sleepFunc; sleeper != nil {
            sleeper(100)
        } else {
            // default small sleep of 100ms
            defaultSleep(100)
        }
        resp, err = s.Client.CreateChatCompletion(ctx, req)
        if err != nil {
            return "", fmt.Errorf("synthesis call (after retry): %w", err)
        }
    }
    if len(resp.Choices) == 0 {
        return "", ErrNoSubstantiveBody
    }
    if s.Verbose {
        // Respect CoT redaction policy; surface only final content unless explicitly enabled
        safe := llmtools.ContentForLogging(resp, s.AllowCOTLogging)
        _ = safe // placeholder in case of future structured logs; no direct print here
    }
    out := strings.TrimSpace(resp.Choices[0].Message.Content)
    if out == "" {
        return "", ErrNoSubstantiveBody
    }
    if s.Cache != nil {
        payload, _ := json.Marshal(map[string]string{"markdown": out})
        _ = s.Cache.Save(ctx, cache.KeyFrom(in.Model, system+"\n\n"+user), payload)
    }
    return out, nil
}

func buildSystemMessage(b brief.Brief) string {
    // Use template-specific system prompt if available
    profile := template.GetProfile(b.ReportType)
    if profile.SystemPrompt != "" {
        return profile.SystemPrompt
    }
    // Fallback to default
    return "You are a careful technical writer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Keep style concise and factual."
}

func buildUserMessage(in Input) string {
    var sb strings.Builder
    sb.WriteString("Write a single cohesive Markdown document with:")
    sb.WriteString("\n- A title on the first line")
    sb.WriteString("\n- A date below the title in ISO format (YYYY-MM-DD)")
    sb.WriteString("\n- An executive summary")
    if len(in.Outline) > 0 {
        sb.WriteString("\n- Body sections matching this outline, in order:")
        for _, h := range in.Outline {
            sb.WriteString("\n  - ")
            sb.WriteString(h)
        }
    }
    // Explicitly require a short section analyzing alternatives and conflicting evidence
    sb.WriteString("\n- An 'Alternatives & conflicting evidence' section that briefly summarizes viable alternatives, known limitations, and any contrary findings from the provided sources")
    sb.WriteString("\n- A 'Risks and limitations' section")
    sb.WriteString("\n- A 'References' section listing all sources as a numbered list with titles and full URLs")
    sb.WriteString("\n- An 'Evidence check' appendix summarizing key claims with supporting source indices and confidence")
    
    // Add template-specific user prompt hint
    profile := template.GetProfile(in.Brief.ReportType)
    if profile.UserPromptHint != "" {
        sb.WriteString("\n\nStructure guidance: ")
        sb.WriteString(profile.UserPromptHint)
    }
    
    if in.LanguageHint != "" {
        sb.WriteString("\nWrite in language: ")
        sb.WriteString(in.LanguageHint)
    }
    sb.WriteString("\n\nBrief topic: ")
    sb.WriteString(in.Brief.Topic)
    if in.Brief.AudienceHint != "" {
        sb.WriteString("\nAudience: ")
        sb.WriteString(in.Brief.AudienceHint)
    }
    if in.Brief.ToneHint != "" {
        sb.WriteString("\nTone: ")
        sb.WriteString(in.Brief.ToneHint)
    }
    if in.Brief.TargetLengthWords > 0 {
        sb.WriteString("\nTarget length: ")
        sb.WriteString(fmt.Sprintf("%d words", in.Brief.TargetLengthWords))
    }
    sb.WriteString("\n\nSources (use only these; cite with [n]):\n")
    for _, src := range in.Sources {
        // Each source begins with its numbered header, then an excerpt block.
        sb.WriteString(fmt.Sprintf("%d. %s — %s\n", src.Index, src.Title, src.URL))
        if strings.TrimSpace(src.Excerpt) != "" {
            sb.WriteString("Excerpt:\n\n")
            sb.WriteString(src.Excerpt)
            sb.WriteString("\n\n")
        }
    }
    sb.WriteString("\nOutput only the Markdown. Do not include any prose outside the document.")
    return sb.String()
}

// sleepFunc allows tests to inject a deterministic sleep hook measured in milliseconds.
// When nil, defaultSleep is used.
var sleepFunc func(ms int)

func defaultSleep(ms int) {
    time.Sleep(time.Duration(ms) * time.Millisecond)
}


