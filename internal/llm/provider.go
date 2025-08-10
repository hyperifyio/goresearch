package llm

import (
    "context"

    openai "github.com/sashabaranov/go-openai"
)

// Client is the minimal interface needed by core logic to call a chat model.
// It intentionally mirrors the CreateChatCompletion method used throughout the
// codebase so that any OpenAI-compatible or local backend can be adapted.
type Client interface {
    CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// ModelLister is an optional capability that allows listing available models.
// Providers that do not support this can omit it; callers should use a type
// assertion to detect availability.
type ModelLister interface {
    ListModels(ctx context.Context) (openai.ModelsList, error)
}

// OpenAIProvider adapts *openai.Client to the Client/ModelLister interfaces.
type OpenAIProvider struct {
    Inner *openai.Client
}

func (p *OpenAIProvider) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
    return p.Inner.CreateChatCompletion(ctx, request)
}

func (p *OpenAIProvider) ListModels(ctx context.Context) (openai.ModelsList, error) {
    return p.Inner.ListModels(ctx)
}
