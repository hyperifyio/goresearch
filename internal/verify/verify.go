package verify

import (
    "context"
    "encoding/json"
    "fmt"
    "regexp"
    "sort"
    "strings"

    openai "github.com/sashabaranov/go-openai"

    "github.com/hyperifyio/goresearch/internal/cache"
)

// ChatClient mirrors the subset we need from the OpenAI client for testability.
type ChatClient interface {
    CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// Claim represents a single extracted claim and its support assessment.
type Claim struct {
    Text       string `json:"text"`
    Citations  []int  `json:"citations"`
    Confidence string `json:"confidence"` // e.g., "high", "medium", "low"
    Supported  bool   `json:"supported"`
}

// Result contains the verification output.
type Result struct {
    Claims  []Claim `json:"claims"`
    Summary string  `json:"summary"`
}

// Verifier performs a secondary model pass (with deterministic fallback) to
// extract claims, map citations, and assess support strength.
type Verifier struct {
    Client ChatClient
    Cache  *cache.LLMCache
    // SystemPrompt, when non-empty, overrides the default verifier system message.
    SystemPrompt string
}

// Verify analyzes the provided Markdown report and returns a verification
// result. If the LLM call is unavailable or fails, a deterministic fallback is
// used to ensure progress.
func (v *Verifier) Verify(ctx context.Context, markdown string, model string, languageHint string) (Result, error) {
    // Try LLM path when configured
    if v.Client != nil && strings.TrimSpace(model) != "" {
        sys := buildSystemMessage()
        if strings.TrimSpace(v.SystemPrompt) != "" {
            sys = v.SystemPrompt
        }
        user := buildUserMessage(markdown, languageHint)
        // Cache lookup
        if v.Cache != nil {
            key := cache.KeyFrom(model, sys+"\n\n"+user)
            if raw, ok, _ := v.Cache.Get(ctx, key); ok {
                var res Result
                if err := json.Unmarshal(raw, &res); err == nil && len(res.Claims) > 0 {
                    return normalizeResult(res), nil
                }
            }
        }
        req := openai.ChatCompletionRequest{
            Model: model,
            Messages: []openai.ChatCompletionMessage{
                {Role: openai.ChatMessageRoleSystem, Content: sys},
                {Role: openai.ChatMessageRoleUser, Content: user},
            },
            Temperature: 0.0,
            N:           1,
        }
        resp, err := v.Client.CreateChatCompletion(ctx, req)
        if err == nil && len(resp.Choices) > 0 {
            raw := strings.TrimSpace(resp.Choices[0].Message.Content)
            var res Result
            if err := json.Unmarshal([]byte(raw), &res); err == nil && len(res.Claims) > 0 {
                res = normalizeResult(res)
                if v.Cache != nil {
                    if b, err := json.Marshal(res); err == nil {
                        _ = v.Cache.Save(ctx, cache.KeyFrom(model, sys+"\n\n"+user), b)
                    }
                }
                return res, nil
            }
        }
        // fall through on any LLM/parse failure
    }
    // Deterministic fallback
    return fallbackVerify(markdown), nil
}

func buildSystemMessage() string {
    return "You are a fact-check verifier. Respond with strict JSON only: {\"claims\":[{\"text\":string,\"citations\":int[],\"confidence\":\"high|medium|low\",\"supported\":bool}],\"summary\":string}. Extract 5-12 key factual claims. Map citations by numeric indices like [3]. If a claim lacks sufficient citation support, mark supported=false and set confidence=low."
}

func buildUserMessage(markdown string, languageHint string) string {
    var sb strings.Builder
    sb.WriteString("Analyze the following Markdown report. Extract key factual claims, map minimal supporting source indices, and assess confidence.\n")
    if strings.TrimSpace(languageHint) != "" {
        sb.WriteString("Language hint: ")
        sb.WriteString(languageHint)
        sb.WriteString("\n")
    }
    sb.WriteString("Report:\n\n")
    sb.WriteString(markdown)
    return sb.String()
}

var citeRe = regexp.MustCompile(`\[(\d+)\]`)

// fallbackVerify implements a deterministic claim extraction:
// - splits into sentences
// - keeps sentences with at least one letter and 8+ words as claims
// - maps inline [n] citations
// - confidence: >=2 cites=high, 1 cite=medium, 0 cite=low (unsupported)
func fallbackVerify(markdown string) Result {
    sentences := splitIntoSentences(markdown)
    claims := make([]Claim, 0, 12)
    for _, s := range sentences {
        text := strings.TrimSpace(s)
        if !looksLikeSentence(text) {
            continue
        }
        cites := parseCitations(text)
        confidence := "low"
        supported := false
        switch {
        case len(cites) >= 2:
            confidence = "high"
            supported = true
        case len(cites) == 1:
            confidence = "medium"
            supported = true
        default:
            confidence = "low"
            supported = false
        }
        claims = append(claims, Claim{Text: text, Citations: cites, Confidence: confidence, Supported: supported})
        if len(claims) >= 12 {
            break
        }
    }
    // prefer claims with citations first
    sort.SliceStable(claims, func(i, j int) bool {
        return len(claims[i].Citations) > len(claims[j].Citations)
    })
    summary := summarizeClaims(claims)
    return Result{Claims: claims, Summary: summary}
}

func parseCitations(s string) []int {
    matches := citeRe.FindAllStringSubmatch(s, -1)
    seen := map[int]struct{}{}
    var out []int
    for _, m := range matches {
        if len(m) != 2 {
            continue
        }
        n := 0
        for _, ch := range m[1] {
            n = n*10 + int(ch-'0')
        }
        if _, ok := seen[n]; ok {
            continue
        }
        seen[n] = struct{}{}
        out = append(out, n)
    }
    sort.Ints(out)
    return out
}

func splitIntoSentences(s string) []string {
    // Naive split on period, newline, or question/exclamation marks.
    // Keep it deterministic and simple; downstream sorting prioritizes cited lines.
    sep := func(r rune) bool {
        return r == '.' || r == '\n' || r == '?' || r == '!'
    }
    raw := strings.FieldsFunc(s, sep)
    out := make([]string, 0, len(raw))
    for _, part := range raw {
        p := strings.TrimSpace(part)
        if p != "" {
            out = append(out, p)
        }
    }
    return out
}

func looksLikeSentence(s string) bool {
    // Require at least some letters and about 8+ words to avoid headings/labels.
    letters := 0
    words := 0
    inWord := false
    for _, r := range s {
        if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
            letters++
        }
        if r == ' ' || r == '\t' {
            if inWord {
                words++
                inWord = false
            }
        } else {
            inWord = true
        }
    }
    if inWord {
        words++
    }
    return letters >= 10 && words >= 8
}

func summarizeClaims(claims []Claim) string {
    total := len(claims)
    if total == 0 {
        return "No extractable claims found."
    }
    supported := 0
    low := 0
    for _, c := range claims {
        if c.Supported {
            supported++
        }
        if c.Confidence == "low" {
            low++
        }
    }
    return fmt.Sprintf("%d claims extracted; %d supported by citations; %d low-confidence.", total, supported, low)
}

func normalizeResult(r Result) Result {
    // Trim text and sort citations for stability
    for i := range r.Claims {
        r.Claims[i].Text = strings.TrimSpace(r.Claims[i].Text)
        sort.Ints(r.Claims[i].Citations)
    }
    return r
}


