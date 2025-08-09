package synth

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "strings"

    openai "github.com/sashabaranov/go-openai"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/cache"
)

// ChatClient abstracts the OpenAI client dependency for testability.
type ChatClient interface {
    CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

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
    Client ChatClient
    Cache  *cache.LLMCache
    Verbose bool
}

// Synthesize requests a single, cohesive Markdown document following the
// structure and citation rules. It returns the raw Markdown string.
func (s *Synthesizer) Synthesize(ctx context.Context, in Input) (string, error) {
    if s.Client == nil || strings.TrimSpace(in.Model) == "" {
        return "", errors.New("synthesizer not configured")
    }
    system := buildSystemMessage()
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

    req := openai.ChatCompletionRequest{
        Model: in.Model,
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleSystem, Content: system},
            {Role: openai.ChatMessageRoleUser, Content: user},
        },
        Temperature: 0.1,
        N:           1,
    }
    resp, err := s.Client.CreateChatCompletion(ctx, req)
    if err != nil {
        return "", fmt.Errorf("synthesis call: %w", err)
    }
    if len(resp.Choices) == 0 {
        return "", errors.New("no choices from model")
    }
    out := strings.TrimSpace(resp.Choices[0].Message.Content)
    if out == "" {
        return "", errors.New("empty synthesis output")
    }
    if s.Cache != nil {
        payload, _ := json.Marshal(map[string]string{"markdown": out})
        _ = s.Cache.Save(ctx, cache.KeyFrom(in.Model, system+"\n\n"+user), payload)
    }
    return out, nil
}

func buildSystemMessage() string {
    // Keep concise but explicit. We rely on validation after generation.
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
    sb.WriteString("\n- A 'Risks and limitations' section")
    sb.WriteString("\n- A 'References' section listing all sources as a numbered list with titles and full URLs")
    sb.WriteString("\n- An 'Evidence check' appendix summarizing key claims with supporting source indices and confidence")
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


