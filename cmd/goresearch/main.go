package main

import (
    "context"
    "fmt"
    "os"
    "flag"
    "time"
    "strings"
    "path/filepath"
    "errors"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"

    "github.com/hyperifyio/goresearch/internal/app"
    "github.com/hyperifyio/goresearch/internal/synth"
)

func main() {
    // Logging setup
    // Keep legacy console writer settings from previous implementation.
    log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

    // Load dotenv files early so env defaults in flag parsing can see them.
    // Non-fatal if files are missing.
    _ = app.LoadEnvFiles(".env", ".env.local")

    // Subcommand: goresearch init
    if len(os.Args) > 1 && os.Args[1] == "init" {
        if err := initScaffold("."); err != nil {
            log.Error().Err(err).Msg("init scaffold failed")
            os.Exit(2)
        }
        log.Info().Msg("created goresearch.yaml and .env.example")
        return
    }

    cfg, verbose, err := parseConfig(os.Args[1:], os.Getenv)
    if err != nil {
        log.Error().Err(err).Msg("parse flags failed")
        os.Exit(2)
    }
    // Single-file config discovery: prefer goresearch.yaml then goresearch.json in cwd
    if _, statErr := os.Stat("goresearch.yaml"); statErr == nil {
        if fc, err := app.LoadConfigFile("goresearch.yaml"); err == nil {
            app.ApplyFileConfig(&cfg, fc)
        } else {
            log.Error().Err(err).Msg("failed to parse goresearch.yaml")
            os.Exit(2)
        }
    } else if _, statErr := os.Stat("goresearch.json"); statErr == nil {
        if fc, err := app.LoadConfigFile("goresearch.json"); err == nil {
            app.ApplyFileConfig(&cfg, fc)
        } else {
            log.Error().Err(err).Msg("failed to parse goresearch.json")
            os.Exit(2)
        }
    }

    // Populate unset fields from environment (after flags and file), so explicit flags win.
    app.ApplyEnvToConfig(&cfg)
    // Then apply env overrides to force env > file when explicitly set
    app.ApplyEnvOverrides(&cfg)
    if verbose {
        zerolog.SetGlobalLevel(zerolog.DebugLevel)
    } else {
        zerolog.SetGlobalLevel(zerolog.InfoLevel)
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

// initScaffold writes a starter goresearch.yaml and .env.example if they don't exist.
func initScaffold(dir string) error {
    // Create goresearch.yaml if missing
    cfgPath := filepath.Join(dir, "goresearch.yaml")
    if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
        sample := []byte(`# goresearch configuration
input: request.md
output: report.md

llm:
  base: ${LLM_BASE_URL}
  model: ${LLM_MODEL}
  key: ${LLM_API_KEY}

searx:
  url: ${SEARX_URL}
  key: ${SEARX_KEY}
  ua: goresearch/1.0 (+https://github.com/hyperifyio/goresearch)

max:
  sources: 12
  perDomain: 3
  perSourceChars: 12000

cache:
  dir: .goresearch-cache
  maxAge: 0s
  clear: false
  strictPerms: false
`)
        if err := os.WriteFile(cfgPath, sample, 0o644); err != nil {
            return fmt.Errorf("write goresearch.yaml: %w", err)
        }
    }
    // Create .env.example if missing
    envExample := filepath.Join(dir, ".env.example")
    if _, err := os.Stat(envExample); errors.Is(err, os.ErrNotExist) {
        sampleEnv := []byte(`LLM_BASE_URL=http://localhost:11434/v1
LLM_MODEL=gpt-oss
LLM_API_KEY=changeme

SEARX_URL=http://localhost:8888
SEARX_KEY=

CACHE_DIR=.goresearch-cache
LANGUAGE=
# SOURCE_CAPS format: <max>[,<perDomain>]
SOURCE_CAPS=12,3
`)
        if err := os.WriteFile(envExample, sampleEnv, 0o644); err != nil {
            return fmt.Errorf("write .env.example: %w", err)
        }
    }
    return nil
}

// parseConfig parses CLI flags and environment variables into app.Config.
// It is separated to enable unit testing of flag behavior.
func parseConfig(args []string, getenv func(string) string) (app.Config, bool, error) {
    fs := flag.NewFlagSet("goresearch", flag.ContinueOnError)
    // Suppress default output during tests; callers can override
    fs.SetOutput(os.Stderr)

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
        // Tools flags
        toolsEnabled       bool
        toolsDryRun        bool
        toolsMaxCalls      int
        toolsMaxWallClock  time.Duration
        toolsPerToolTimeout time.Duration
        toolsMode          string
    )

    fs.StringVar(&inputPath, "input", "request.md", "Path to input Markdown research request")
    fs.StringVar(&outputPath, "output", "report.md", "Path to write the final Markdown report")
    fs.StringVar(&searxURL, "searx.url", getenv("SEARX_URL"), "SearxNG base URL")
    fs.StringVar(&searxKey, "searx.key", getenv("SEARX_KEY"), "SearxNG API key (optional)")
    fs.StringVar(&searxUA, "searx.ua", "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)", "Custom User-Agent for SearxNG requests")
    fs.StringVar(&fileSearchPath, "search.file", getenv("SEARCH_FILE"), "Path to JSON file for offline file-based search provider")
    fs.StringVar(&llmBaseURL, "llm.base", getenv("LLM_BASE_URL"), "OpenAI-compatible base URL")
    fs.StringVar(&llmModel, "llm.model", getenv("LLM_MODEL"), "Model name")
    fs.StringVar(&llmKey, "llm.key", getenv("LLM_API_KEY"), "API key for OpenAI-compatible server")
    fs.IntVar(&maxSources, "max.sources", 12, "Maximum number of sources")
    fs.IntVar(&perDomain, "max.perDomain", 3, "Maximum sources per domain")
    fs.IntVar(&perSourceChars, "max.perSourceChars", 12000, "Maximum characters per source extract")
    fs.IntVar(&minSnippetChars, "min.snippetChars", 0, "Minimum non-whitespace snippet characters to keep a result (0 disables)")
    fs.StringVar(&language, "lang", "", "Optional language hint, e.g. 'en' or 'fi'")
    fs.BoolVar(&dryRun, "dry-run", false, "Plan and select without calling the model")
    fs.BoolVar(&verbose, "v", false, "Verbose logging")
    fs.BoolVar(&debugVerbose, "debug-verbose", false, "Allow logging raw chain-of-thought (CoT) for debugging Harmony/tool-call interplay")
    fs.StringVar(&cacheDir, "cache.dir", ".goresearch-cache", "Cache directory path")
    fs.DurationVar(&cacheMaxAge, "cache.maxAge", 0, "Max age for cache entries before purge (e.g. 24h, 7d); 0 disables")
    fs.BoolVar(&cacheClear, "cache.clear", false, "Clear cache directory before run")
    fs.BoolVar(&cacheStrict, "cache.strictPerms", false, "Restrict cache permissions (0700 dirs, 0600 files)")
    fs.StringVar(&topicHash, "cache.topicHash", getenv("TOPIC_HASH"), "Optional topic hash to scope cache; accepted for traceability")
    fs.BoolVar(&enablePDF, "enable.pdf", false, "Enable optional PDF ingestion (application/pdf)")
    // Prompt profile flexibility: allow overriding system prompts via flags/env
    fs.StringVar(&synthSystemPrompt, "synth.systemPrompt", getenv("SYNTH_SYSTEM_PROMPT"), "Override synthesis system prompt (inline string)")
    fs.StringVar(&synthSystemPromptFile, "synth.systemPromptFile", getenv("SYNTH_SYSTEM_PROMPT_FILE"), "Path to file containing synthesis system prompt")
    fs.StringVar(&verifySystemPrompt, "verify.systemPrompt", getenv("VERIFY_SYSTEM_PROMPT"), "Override verification system prompt (inline string)")
    fs.StringVar(&verifySystemPromptFile, "verify.systemPromptFile", getenv("VERIFY_SYSTEM_PROMPT_FILE"), "Path to file containing verification system prompt")
    // Robots override (bounded allowlist + explicit confirm)
    fs.StringVar(&robotsOverrideAllowlist, "robots.overrideDomains", getenv("ROBOTS_OVERRIDE_DOMAINS"), "Comma-separated domain allowlist to ignore robots.txt (use with --robots.overrideConfirm)")
    fs.BoolVar(&robotsOverrideConfirm, "robots.overrideConfirm", false, "Second confirmation flag required to activate robots override allowlist")
    // Centralized domain allow/deny lists
    fs.StringVar(&domainsAllow, "domains.allow", getenv("DOMAINS_ALLOW"), "Comma-separated allowlist of hosts/domains; if set, only these are permitted (subdomains included)")
    fs.StringVar(&domainsDeny, "domains.deny", getenv("DOMAINS_DENY"), "Comma-separated denylist of hosts/domains; takes precedence over allow")
    // Tools orchestration flags (config flags item)
    fs.BoolVar(&toolsEnabled, "tools.enable", false, "Enable tool-orchestrated chat mode")
    fs.BoolVar(&toolsDryRun, "tools.dryRun", false, "Do not execute tools; emit dry-run envelopes")
    fs.IntVar(&toolsMaxCalls, "tools.maxCalls", 32, "Max tool calls per run")
    fs.DurationVar(&toolsMaxWallClock, "tools.maxWallClock", 0, "Max wall-clock duration for tool loop (e.g. 30s); 0 disables")
    fs.DurationVar(&toolsPerToolTimeout, "tools.perToolTimeout", 10*time.Second, "Per-tool execution timeout (e.g. 10s)")
    fs.StringVar(&toolsMode, "tools.mode", "harmony", "Chat protocol mode: harmony|legacy")

    if err := fs.Parse(args); err != nil {
        return app.Config{}, false, err
    }
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

    cfg := app.Config{
        InputPath:       inputPath,
        OutputPath:      outputPath,
        SearxURL:        searxURL,
        SearxKey:        searxKey,
        SearxUA:         searxUA,
        FileSearchPath:  fileSearchPath,
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
        ToolsEnabled:    toolsEnabled,
        ToolsDryRun:     toolsDryRun,
        ToolsMaxCalls:   toolsMaxCalls,
        ToolsMaxWallClock: toolsMaxWallClock,
        ToolsPerToolTimeout: toolsPerToolTimeout,
        ToolsMode:       toolsMode,
    }

    // Parse robots override domains into slice
    if s := strings.TrimSpace(robotsOverrideAllowlist); s != "" {
        parts := strings.Split(s, ",")
        list := make([]string, 0, len(parts))
        for _, p := range parts {
            if v := strings.TrimSpace(p); v != "" { list = append(list, v) }
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
    return cfg, verbose, nil
}

// isNoSubstantiveBody checks whether the error indicates the synthesizer
// produced no substantive output. We keep it narrow to avoid masking real
// failures under the exit-code policy.
func isNoSubstantiveBody(err error) bool {
    return err == synth.ErrNoSubstantiveBody
}

func run(cfg app.Config) error {
	ctx := context.Background()

    // Validate final config before starting
    if err := app.ValidateConfig(cfg); err != nil {
        return fmt.Errorf("invalid configuration: %w", err)
    }

	a, err := app.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	return a.Run(ctx)
}
