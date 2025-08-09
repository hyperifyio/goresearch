package app

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
}
