package app

import "time"

// Config holds runtime configuration for the application.
type Config struct {
	InputPath  string
	OutputPath string

	// Search
	SearxURL string
	SearxKey string
    SearxUA  string
    FileSearchPath string

	// LLM
	LLMBaseURL string
	LLMModel   string
	LLMAPIKey  string
    // Prompt overrides
    SynthSystemPrompt  string
    VerifySystemPrompt string

	// Selection / budgeting
	MaxSources           int
	PerDomainCap         int
	PerSourceChars       int
	LanguageHint         string
	MinSnippetChars      int
	ReservedOutputTokens int

	// Behavior
	DryRun   bool
	CacheDir string
	Verbose  bool
    // CacheStrictPerms, when true, uses 0700 for cache dirs and 0600 for files.
    CacheStrictPerms bool

    // EnablePDF gates optional PDF ingestion. When true, the fetcher will accept
    // application/pdf content types and the extractor will attempt to parse text
    // from PDFs. Default is false to avoid binary parsing risk by default.
    EnablePDF bool

	// Cache invalidation controls
	// If > 0, remove cache entries older than this age before running.
	CacheMaxAge time.Duration
	// If true, clear the entire cache directory before running.
	CacheClear bool
	// Optional topic hash for selective invalidation in the future. For now,
	// accepted and logged for traceability.
	TopicHash string

    // AllowPrivateHosts relaxes the public-web guard in the fetcher so that
    // localhost and private IPs can be used during tests and local fixtures.
    // Defaults to false in production.
    AllowPrivateHosts bool
}
