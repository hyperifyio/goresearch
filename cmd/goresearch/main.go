package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
    "strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/hyperifyio/goresearch/internal/app"
	"github.com/hyperifyio/goresearch/internal/synth"
)

func main() {
	// Logging setup
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	var (
		inputPath       string
		outputPath      string
		searxURL        string
		searxKey        string
		searxUA         string
		fileSearchPath  string
		llmBaseURL      string
		llmModel        string
		llmKey          string
		maxSources      int
		perDomain       int
		perSourceChars  int
		minSnippetChars int
		language        string
		dryRun          bool
    	verbose         bool
    	debugVerbose    bool
		cacheDir        string
		cacheMaxAge     time.Duration
		cacheClear      bool
		cacheStrict     bool
		topicHash       string
		enablePDF       bool
	    synthSystemPrompt     string
	    synthSystemPromptFile string
	    verifySystemPrompt    string
	    verifySystemPromptFile string
	    robotsOverrideAllowlist string
	    robotsOverrideConfirm   bool
		domainsAllow            string
		domainsDeny             string
	)

	flag.StringVar(&inputPath, "input", "request.md", "Path to input Markdown research request")
	flag.StringVar(&outputPath, "output", "report.md", "Path to write the final Markdown report")
	flag.StringVar(&searxURL, "searx.url", os.Getenv("SEARX_URL"), "SearxNG base URL")
	flag.StringVar(&searxKey, "searx.key", os.Getenv("SEARX_KEY"), "SearxNG API key (optional)")
	flag.StringVar(&searxUA, "searx.ua", "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)", "Custom User-Agent for SearxNG requests")
	flag.StringVar(&fileSearchPath, "search.file", os.Getenv("SEARCH_FILE"), "Path to JSON file for offline file-based search provider")
	flag.StringVar(&llmBaseURL, "llm.base", os.Getenv("LLM_BASE_URL"), "OpenAI-compatible base URL")
	flag.StringVar(&llmModel, "llm.model", os.Getenv("LLM_MODEL"), "Model name")
	flag.StringVar(&llmKey, "llm.key", os.Getenv("LLM_API_KEY"), "API key for OpenAI-compatible server")
	flag.IntVar(&maxSources, "max.sources", 12, "Maximum number of sources")
	flag.IntVar(&perDomain, "max.perDomain", 3, "Maximum sources per domain")
	flag.IntVar(&perSourceChars, "max.perSourceChars", 12000, "Maximum characters per source extract")
	flag.IntVar(&minSnippetChars, "min.snippetChars", 0, "Minimum non-whitespace snippet characters to keep a result (0 disables)")
	flag.StringVar(&language, "lang", "", "Optional language hint, e.g. 'en' or 'fi'")
	flag.BoolVar(&dryRun, "dry-run", false, "Plan and select without calling the model")
    flag.BoolVar(&verbose, "v", false, "Verbose logging")
    flag.BoolVar(&debugVerbose, "debug-verbose", false, "Allow logging raw chain-of-thought (CoT) for debugging Harmony/tool-call interplay")
	flag.StringVar(&cacheDir, "cache.dir", ".goresearch-cache", "Cache directory path")
	flag.DurationVar(&cacheMaxAge, "cache.maxAge", 0, "Max age for cache entries before purge (e.g. 24h, 7d); 0 disables")
	flag.BoolVar(&cacheClear, "cache.clear", false, "Clear cache directory before run")
	flag.BoolVar(&cacheStrict, "cache.strictPerms", false, "Restrict cache permissions (0700 dirs, 0600 files)")
	flag.StringVar(&topicHash, "cache.topicHash", os.Getenv("TOPIC_HASH"), "Optional topic hash to scope cache; accepted for traceability")
	flag.BoolVar(&enablePDF, "enable.pdf", false, "Enable optional PDF ingestion (application/pdf)")
	// Prompt profile flexibility: allow overriding system prompts via flags/env
	flag.StringVar(&synthSystemPrompt, "synth.systemPrompt", os.Getenv("SYNTH_SYSTEM_PROMPT"), "Override synthesis system prompt (inline string)")
	flag.StringVar(&synthSystemPromptFile, "synth.systemPromptFile", os.Getenv("SYNTH_SYSTEM_PROMPT_FILE"), "Path to file containing synthesis system prompt")
	flag.StringVar(&verifySystemPrompt, "verify.systemPrompt", os.Getenv("VERIFY_SYSTEM_PROMPT"), "Override verification system prompt (inline string)")
	flag.StringVar(&verifySystemPromptFile, "verify.systemPromptFile", os.Getenv("VERIFY_SYSTEM_PROMPT_FILE"), "Path to file containing verification system prompt")
	// Robots override (bounded allowlist + explicit confirm)
	flag.StringVar(&robotsOverrideAllowlist, "robots.overrideDomains", os.Getenv("ROBOTS_OVERRIDE_DOMAINS"), "Comma-separated domain allowlist to ignore robots.txt (use with --robots.overrideConfirm)")
	flag.BoolVar(&robotsOverrideConfirm, "robots.overrideConfirm", false, "Second confirmation flag required to activate robots override allowlist")
	// Centralized domain allow/deny lists
	flag.StringVar(&domainsAllow, "domains.allow", os.Getenv("DOMAINS_ALLOW"), "Comma-separated allowlist of hosts/domains; if set, only these are permitted (subdomains included)")
	flag.StringVar(&domainsDeny, "domains.deny", os.Getenv("DOMAINS_DENY"), "Comma-separated denylist of hosts/domains; takes precedence over allow")
	flag.Parse()
	// If file-based prompts are provided, they take precedence over inline strings
	if strings.TrimSpace(synthSystemPromptFile) != "" {
		if b, err := os.ReadFile(synthSystemPromptFile); err == nil {
			synthSystemPrompt = string(b)
		}
	}
	if strings.TrimSpace(verifySystemPromptFile) != "" {
		if b, err := os.ReadFile(verifySystemPromptFile); err == nil {
			verifySystemPrompt = string(b)
		}
	}

	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	cfg := app.Config{
		InputPath:       inputPath,
		OutputPath:      outputPath,
		SearxURL:        searxURL,
		SearxKey:        searxKey,
		LLMBaseURL:      llmBaseURL,
		LLMModel:        llmModel,
		LLMAPIKey:       llmKey,
		MaxSources:      maxSources,
		PerDomainCap:    perDomain,
		PerSourceChars:  perSourceChars,
		MinSnippetChars: minSnippetChars,
		LanguageHint:    language,
		DryRun:          dryRun,
		CacheDir:        cacheDir,
        Verbose:         verbose,
        DebugVerbose:    debugVerbose,
		CacheMaxAge:     cacheMaxAge,
		CacheClear:      cacheClear,
		CacheStrictPerms: cacheStrict,
		TopicHash:       topicHash,
		EnablePDF:       enablePDF,
		SynthSystemPrompt:  synthSystemPrompt,
		VerifySystemPrompt: verifySystemPrompt,
		RobotsOverrideConfirm: robotsOverrideConfirm,
	}

	// Parse robots override domains into slice
	if strings.TrimSpace(robotsOverrideAllowlist) != "" {
		parts := strings.Split(robotsOverrideAllowlist, ",")
		list := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" { list = append(list, s) }
		}
		cfg.RobotsOverrideAllowlist = list
	}

	// Parse centralized domain allow/deny lists
	if s := strings.TrimSpace(domainsAllow); s != "" {
		parts := strings.Split(s, ",")
		list := make([]string, 0, len(parts))
		for _, p := range parts { if v := strings.TrimSpace(p); v != "" { list = append(list, v) } }
		cfg.DomainAllowlist = list
	}
	if s := strings.TrimSpace(domainsDeny); s != "" {
		parts := strings.Split(s, ",")
		list := make([]string, 0, len(parts))
		for _, p := range parts { if v := strings.TrimSpace(p); v != "" { list = append(list, v) } }
		cfg.DomainDenylist = list
	}

	if err := run(cfg); err != nil {
		log.Error().Err(err).Msg("run failed")
		// Exit code policy: nonzero only on no usable sources or no substantive body.
		// Map known sentinel errors to exit code 2, otherwise exit 0 (warnings).
		if err == app.ErrNoUsableSources || isNoSubstantiveBody(err) {
			os.Exit(2)
		}
		// For other errors, treat as warnings and exit 0 to allow completion with warnings.
		os.Exit(0)
	}
}

// isNoSubstantiveBody checks whether the error indicates the synthesizer
// produced no substantive output. We keep it narrow to avoid masking real
// failures under the exit-code policy.
func isNoSubstantiveBody(err error) bool {
    return err == synth.ErrNoSubstantiveBody
}

func run(cfg app.Config) error {
	ctx := context.Background()

	a, err := app.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	return a.Run(ctx)
}
