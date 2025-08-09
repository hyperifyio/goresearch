package app

import "time"

// Config holds runtime configuration for the application.
type Config struct {
	InputPath  string
	OutputPath string

	// Search
	SearxURL string
	SearxKey string

	// LLM
	LLMBaseURL string
	LLMModel   string
	LLMAPIKey  string

	// Selection / budgeting
	MaxSources     int
	PerDomainCap   int
	PerSourceChars int
	LanguageHint   string
    MinSnippetChars int
    ReservedOutputTokens int

	// Behavior
	DryRun   bool
	CacheDir string
	Verbose  bool

    // Cache invalidation controls
    // If > 0, remove cache entries older than this age before running.
    CacheMaxAge time.Duration
    // If true, clear the entire cache directory before running.
    CacheClear bool
    // Optional topic hash for selective invalidation in the future. For now,
    // accepted and logged for traceability.
    TopicHash string
}
