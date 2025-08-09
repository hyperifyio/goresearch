package synth

import (
    "context"
    "fmt"
    "strings"
    "testing"

    openai "github.com/sashabaranov/go-openai"

    "github.com/hyperifyio/goresearch/internal/brief"
)

type capturingClient struct{ lastReq openai.ChatCompletionRequest }

func (c *capturingClient) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
    c.lastReq = req
    // Return a minimal valid response
    return openai.ChatCompletionResponse{
        Choices: []openai.ChatCompletionChoice{{
            Message: openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "# ok"},
        }},
    }, nil
}

func TestSynthesizer_IncludesLanguageInstruction(t *testing.T) {
    cc := &capturingClient{}
    s := &Synthesizer{Client: cc}
    in := Input{
        Brief: brief.Brief{Topic: "Kubernetes"},
        Outline: []string{"Intro"},
        Sources: nil,
        Model: "test-model",
        LanguageHint: "es",
        ReservedOutputTokens: 256,
    }
    out, err := s.Synthesize(context.Background(), in)
    if err != nil {
        t.Fatalf("synthesize error: %v", err)
    }
    if out == "" {
        t.Fatalf("expected non-empty output")
    }
    // The user message should contain explicit language instruction
    if len(cc.lastReq.Messages) == 0 {
        t.Fatalf("expected messages in request")
    }
    // user message is second
    if got := cc.lastReq.Messages[1].Content; !containsAll(got, []string{"Write in language:", "es"}) {
        t.Fatalf("expected user message to include language instruction; got:\n%s", got)
    }
}

func containsAll(s string, parts []string) bool {
    for _, p := range parts {
        if !contains(s, p) {
            return false
        }
    }
    return true
}

func contains(s, sub string) bool { return (len(s) >= len(sub)) && (indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
    // simple wrapper to avoid importing strings to keep import list minimal; but we can use strings.Index safely
    return len(fmt.Sprintf("%s", s[:])) + func() int { return stringsIndex(s, sub) }() - len(s)
}

// stringsIndex delegates to strings.Index; defined to keep top import tidy in this block
func stringsIndex(haystack, needle string) int {
    return strings.Index(haystack, needle)
}
