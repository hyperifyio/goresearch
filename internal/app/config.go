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
    // HTTPCacheOnly, when true, serves HTTP bodies exclusively from the
    // on-disk HTTP cache and fails fast on a cache miss. No network requests
    // are attempted. Intended for offline/airgapped operation.
    HTTPCacheOnly bool
    // LLMCacheOnly, when true, serves planner/synthesizer/verifier results
    // exclusively from the LLM cache and fails fast on a cache miss. No LLM
    // requests are attempted. Intended for offline/airgapped operation.
    LLMCacheOnly bool
    // DebugVerbose, when true, allows logging of raw chain-of-thought (CoT)
    // content for debugging Harmony/tool-call interplay. Default is false and
    // CoT is redacted from logs. Use sparingly.
    DebugVerbose bool
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

    // Robots override: when enabled via confirmation, ignore robots.txt for
    // hosts listed in RobotsOverrideAllowlist. Intended for bounded internal
    // mirrors or explicitly permitted domains.
    RobotsOverrideAllowlist []string
    RobotsOverrideConfirm   bool

    // Domain allow/deny lists — evaluated before any networked operation.
    // When Denylist contains a host or parent domain, access is blocked.
    // When Allowlist is non-empty, only listed hosts/domains are permitted.
    // Denylist takes precedence over Allowlist.
    DomainAllowlist []string
    DomainDenylist  []string

    // Tools / Orchestration
    // ToolsEnabled toggles the tool-orchestrated research mode.
    ToolsEnabled bool
    // ToolsDryRun, when true, does not execute tool handlers; instead,
    // structured dry-run envelopes are produced to trace intended calls.
    ToolsDryRun bool
    // ToolsMaxCalls bounds the total number of tool calls during one run.
    ToolsMaxCalls int
    // ToolsMaxWallClock bounds the total wall-clock duration of the
    // orchestration loop. Zero disables the additional deadline.
    ToolsMaxWallClock time.Duration
    // ToolsPerToolTimeout bounds the duration of a single tool execution.
    ToolsPerToolTimeout time.Duration
    // ToolsMode selects chat protocol nuances: "harmony" (default) uses
    // Harmony-style analysis/commentary/final markers; "legacy" treats
    // assistant content as final without requiring Harmony markers.
    ToolsMode string
}
