package main

import (
    "bytes"
    "context"
    "errors"
    "flag"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "reflect"
    "sort"
    "strings"
    "time"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"

    "github.com/hyperifyio/goresearch/internal/app"
    "github.com/hyperifyio/goresearch/internal/synth"
)

func main() {
    // Logging setup
    // Keep legacy console writer settings from previous implementation.
    log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
    // Ensure all timestamps are recorded in UTC.
    zerolog.TimestampFunc = func() time.Time { return time.Now().UTC() }

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

    // Subcommand: goresearch doc — print CLI/options reference in Markdown
    if len(os.Args) > 1 && os.Args[1] == "doc" {
        fs := buildDocFlagSet(os.Getenv)
        md := renderCLIReferenceMarkdown(fs)
        fmt.Print(md)
        return
    }

    // Subcommand: goresearch version — print version/build information
    if len(os.Args) > 1 && os.Args[1] == "version" {
        fmt.Print(renderVersion())
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
    // Configure logging: concise console output to stderr; structured JSON to file.
    // Level precedence: --log.level > -v (debug) > info default.
    level := zerolog.InfoLevel
    if strings.TrimSpace(cfg.LogLevel) != "" {
        if lv, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(cfg.LogLevel))); err == nil {
            level = lv
        } else {
            log.Warn().Str("log.level", cfg.LogLevel).Msg("unknown log level; defaulting to info")
        }
    } else if verbose {
        level = zerolog.DebugLevel
    }
    zerolog.SetGlobalLevel(level)

    // Open log file for structured logs (JSON). Default path when unset.
    logPath := strings.TrimSpace(cfg.LogFilePath)
    if logPath == "" { logPath = filepath.ToSlash(filepath.Join("logs", "goresearch.log")) }
    // Ensure parent directory exists for the log file path
    if dir := filepath.Dir(logPath); strings.TrimSpace(dir) != "" && dir != "." {
        _ = os.MkdirAll(dir, 0o755)
    }
    var file io.Writer
    if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
        file = f
    } else {
        log.Warn().Err(err).Str("log_file", logPath).Msg("cannot open log file; continuing without file logs")
    }
    console := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
    console.NoColor = false
    // Ensure UTC in console timestamps as well
    console.TimeFormat = time.RFC3339
    if file != nil {
        // Route logs to both console and file using a MultiWriter.
        // Console formatting is applied only to stderr; file receives plain JSON entries.
        mw := io.MultiWriter(console, file)
        log.Logger = zerolog.New(mw).With().Timestamp().Logger()
    } else {
        log.Logger = zerolog.New(console).With().Timestamp().Logger()
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
        sampleEnv := []byte(`# Copy to .env and edit values. Do not commit your .env.
# Required runtime variables
LLM_BASE_URL=http://localhost:11434/v1
LLM_MODEL=gpt-oss

# SearxNG base URL (either SEARX_URL or SEARXNG_URL is accepted)
SEARXNG_URL=http://localhost:8888
# SEARX_URL=http://localhost:8888

# Optional but recommended
CACHE_DIR=.goresearch-cache
LANGUAGE=
# SOURCE_CAPS format: <max>[,<perDomain>]
SOURCE_CAPS=12,3

# Secrets — provide via environment or .env (never commit secrets)
LLM_API_KEY=
SEARX_KEY=
`)
        if err := os.WriteFile(envExample, sampleEnv, 0o644); err != nil {
            return fmt.Errorf("write .env.example: %w", err)
        }
    }
    return nil
}

// boundVars groups pointers to variables bound in a constructed FlagSet so we
// can read their values after parsing and also reuse the same specification to
// render documentation.
type boundVars struct {
    inputPath, outputPath                 *string
    searxURL, searxKey, searxUA           *string
    fileSearchPath                        *string
    llmBaseURL, llmModel, llmKey          *string
    maxSources, perDomain, perSourceChars *int
    minSnippetChars                       *int
    language                              *string
    dryRun, verbose, debugVerbose         *bool
    cacheDir                              *string
    cacheMaxAge                           *time.Duration
    cacheClear, cacheStrict               *bool
    sslVerify                             *bool
    topicHash                             *string
    enablePDF                             *bool
    synthSystemPrompt, synthSystemPromptFile *string
    verifySystemPrompt, verifySystemPromptFile *string
    robotsOverrideAllowlist               *string
    robotsOverrideConfirm                 *bool
    domainsAllow, domainsDeny             *string
    toolsEnabled, toolsDryRun             *bool
    toolsMaxCalls                         *int
    toolsMaxWallClock, toolsPerToolTimeout *time.Duration
    toolsMode                              *string
    verifyEnabled                          *bool
    noVerify                               *bool
    reportsDir                              *string
    reportsTar                              *bool
    logLevel                                *string
    logFile                                 *string
}

// flagMeta holds the FlagSet and the pointers to bound variables for later use.
type flagMeta struct {
    fs   *flag.FlagSet
    bound boundVars
}

// newFlagSet constructs the canonical CLI FlagSet and returns it with the bound variable pointers.
func newFlagSet(getenv func(string) string) (*flag.FlagSet, flagMeta) {
    fs := flag.NewFlagSet("goresearch", flag.ContinueOnError)
    // Defaults
    bv := boundVars{}
    bv.inputPath = fs.String("input", "request.md", "Path to input Markdown research request")
    bv.outputPath = fs.String("output", "report.md", "Path to write the final Markdown report")
    bv.searxURL = fs.String("searx.url", getenv("SEARX_URL"), "SearxNG base URL")
    bv.searxKey = fs.String("searx.key", getenv("SEARX_KEY"), "SearxNG API key (optional)")
    bv.searxUA = fs.String("searx.ua", "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)", "Custom User-Agent for SearxNG requests")
    bv.fileSearchPath = fs.String("search.file", getenv("SEARCH_FILE"), "Path to JSON file for offline file-based search provider")
    bv.llmBaseURL = fs.String("llm.base", getenv("LLM_BASE_URL"), "OpenAI-compatible base URL")
    bv.llmModel = fs.String("llm.model", getenv("LLM_MODEL"), "Model name")
    bv.llmKey = fs.String("llm.key", getenv("LLM_API_KEY"), "API key for OpenAI-compatible server")
    bv.maxSources = fs.Int("max.sources", 12, "Maximum number of sources")
    bv.perDomain = fs.Int("max.perDomain", 3, "Maximum sources per domain")
    bv.perSourceChars = fs.Int("max.perSourceChars", 12000, "Maximum characters per source extract")
    bv.minSnippetChars = fs.Int("min.snippetChars", 0, "Minimum non-whitespace snippet characters to keep a result (0 disables)")
    bv.language = fs.String("lang", "", "Optional language hint, e.g. 'en' or 'fi'")
    bv.dryRun = fs.Bool("dry-run", false, "Plan and select without calling the model")
    bv.verbose = fs.Bool("v", false, "Verbose logging")
    bv.debugVerbose = fs.Bool("debug-verbose", false, "Allow logging raw chain-of-thought (CoT) for debugging Harmony/tool-call interplay")
    bv.cacheDir = fs.String("cache.dir", ".goresearch-cache", "Cache directory path")
    bv.cacheMaxAge = fs.Duration("cache.maxAge", 0, "Max age for cache entries before purge (e.g. 24h, 7d); 0 disables")
    bv.cacheClear = fs.Bool("cache.clear", false, "Clear cache directory before run")
    bv.cacheStrict = fs.Bool("cache.strictPerms", false, "Restrict cache permissions (0700 dirs, 0600 files)")
    bv.sslVerify = fs.Bool("ssl.verify", getenv("SSL_VERIFY") != "false", "Enable SSL certificate verification (set to false for self-signed certs)")
    bv.topicHash = fs.String("cache.topicHash", getenv("TOPIC_HASH"), "Optional topic hash to scope cache; accepted for traceability")
    bv.enablePDF = fs.Bool("enable.pdf", false, "Enable optional PDF ingestion (application/pdf)")
    // Prompt overrides
    bv.synthSystemPrompt = fs.String("synth.systemPrompt", getenv("SYNTH_SYSTEM_PROMPT"), "Override synthesis system prompt (inline string)")
    bv.synthSystemPromptFile = fs.String("synth.systemPromptFile", getenv("SYNTH_SYSTEM_PROMPT_FILE"), "Path to file containing synthesis system prompt")
    bv.verifySystemPrompt = fs.String("verify.systemPrompt", getenv("VERIFY_SYSTEM_PROMPT"), "Override verification system prompt (inline string)")
    bv.verifySystemPromptFile = fs.String("verify.systemPromptFile", getenv("VERIFY_SYSTEM_PROMPT_FILE"), "Path to file containing verification system prompt")
    // Robots override & domains
    bv.robotsOverrideAllowlist = fs.String("robots.overrideDomains", getenv("ROBOTS_OVERRIDE_DOMAINS"), "Comma-separated domain allowlist to ignore robots.txt (use with --robots.overrideConfirm)")
    bv.robotsOverrideConfirm = fs.Bool("robots.overrideConfirm", false, "Second confirmation flag required to activate robots override allowlist")
    bv.domainsAllow = fs.String("domains.allow", getenv("DOMAINS_ALLOW"), "Comma-separated allowlist of hosts/domains; if set, only these are permitted (subdomains included)")
    bv.domainsDeny = fs.String("domains.deny", getenv("DOMAINS_DENY"), "Comma-separated denylist of hosts/domains; takes precedence over allow")
    // Tools
    bv.toolsEnabled = fs.Bool("tools.enable", false, "Enable tool-orchestrated chat mode")
    bv.toolsDryRun = fs.Bool("tools.dryRun", false, "Do not execute tools; emit dry-run envelopes")
    bv.toolsMaxCalls = fs.Int("tools.maxCalls", 32, "Max tool calls per run")
    bv.toolsMaxWallClock = fs.Duration("tools.maxWallClock", 0, "Max wall-clock duration for tool loop (e.g. 30s); 0 disables")
    bv.toolsPerToolTimeout = fs.Duration("tools.perToolTimeout", 10*time.Second, "Per-tool execution timeout (e.g. 10s)")
    bv.toolsMode = fs.String("tools.mode", "harmony", "Chat protocol mode: harmony|legacy")
    // Verification toggle (default enabled). Provide both --verify and --no-verify for clarity.
    bv.verifyEnabled = fs.Bool("verify", true, "Enable the fact-check verification pass and Evidence check appendix")
    bv.noVerify = fs.Bool("no-verify", false, "Disable the fact-check verification pass and Evidence check appendix")
    // Artifacts bundle
    bv.reportsDir = fs.String("reports.dir", "reports", "Root directory to persist artifacts bundles (reports)")
    bv.reportsTar = fs.Bool("reports.tar", false, "Also produce a tar.gz of the bundle with digests for offline audit")
    // Logging controls
    bv.logLevel = fs.String("log.level", strings.TrimSpace(getenv("LOG_LEVEL")), "Structured log level for file output: trace|debug|info|warn|error|fatal|panic (default info)")
    bv.logFile = fs.String("log.file", strings.TrimSpace(getenv("LOG_FILE")), "Path to write structured JSON logs (default goresearch.log)")

    return fs, flagMeta{fs: fs, bound: bv}
}

// buildDocFlagSet returns a FlagSet with the same flags as the main CLI but
// with output redirected to a buffer to avoid printing usage during doc render.
func buildDocFlagSet(getenv func(string) string) *flag.FlagSet {
    fs, _ := newFlagSet(getenv)
    // Do not parse; we only need definitions and defaults
    return fs
}

// renderCLIReferenceMarkdown renders a comprehensive CLI/options page from a FlagSet.
func renderCLIReferenceMarkdown(fs *flag.FlagSet) string {
    var b bytes.Buffer
    b.WriteString("# goresearch CLI reference\n\n")
    b.WriteString("This page is auto-generated from the CLI flag definitions.\n\n")
    b.WriteString("## Usage\n\n")
    b.WriteString("```\n")
    b.WriteString("goresearch [flags]\n")
    b.WriteString("goresearch init\n")
    b.WriteString("goresearch doc\n")
    b.WriteString("```\n\n")

    // Collect flags for stable ordering by name
    type row struct{ name, def, usage string }
    rows := make([]row, 0, 96)
    fs.VisitAll(func(f *flag.Flag) {
        rows = append(rows, row{name: f.Name, def: f.DefValue, usage: f.Usage})
    })
    sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

    b.WriteString("## Flags\n\n")
    for _, r := range rows {
        // Render as a definition list-like block
        b.WriteString(fmt.Sprintf("- `-%s` (default: `%s`) — %s\n", r.name, r.def, r.usage))
    }

    b.WriteString("\n## Environment variables\n\n")
    // Reflect environment variables from config_env.go docs via a curated list
    envs := []struct{ key, desc string }{
        {"LLM_BASE_URL", "OpenAI-compatible base URL"},
        {"LLM_MODEL", "Model name"},
        {"LLM_API_KEY", "API key"},
        {"SEARX_URL", "SearxNG base URL (or SEARXNG_URL)"},
        {"SEARX_KEY", "SearxNG API key (or SEARXNG_KEY)"},
        {"CACHE_DIR", "Cache directory path"},
        {"LANGUAGE", "Language hint"},
        {"SOURCE_CAPS", "Max sources and optional per-domain cap as '<max>' or '<max>,<perDomain>'"},
        {"CACHE_MAX_AGE", "Purge cache entries older than this duration (e.g. 24h, 7d)"},
        {"DRY_RUN", "Enable dry-run when truthy"},
        {"VERBOSE", "Enable verbose logs when truthy"},
        {"CACHE_CLEAR", "Clear cache before run when truthy"},
        {"CACHE_STRICT_PERMS", "Restrict cache permissions when truthy"},
        {"SSL_VERIFY", "Enable SSL certificate verification (set to 'false' for self-signed certs)"},
        {"HTTP_CACHE_ONLY", "Serve HTTP bodies only from cache; fail on miss"},
        {"LLM_CACHE_ONLY", "Serve LLM results only from cache; fail on miss"},
        {"ROBOTS_OVERRIDE_DOMAINS", "Comma-separated allowlist to ignore robots.txt; requires robots.overrideConfirm"},
        {"DOMAINS_ALLOW", "Comma-separated allowlist of hosts/domains"},
        {"DOMAINS_DENY", "Comma-separated denylist of hosts/domains"},
        {"SYNTH_SYSTEM_PROMPT", "Inline synthesis system prompt override"},
        {"SYNTH_SYSTEM_PROMPT_FILE", "Path to synthesis system prompt file"},
        {"VERIFY_SYSTEM_PROMPT", "Inline verification system prompt override"},
        {"VERIFY_SYSTEM_PROMPT_FILE", "Path to verification system prompt file"},
        {"TOPIC_HASH", "Optional topic hash to scope cache"},
        {"VERIFY", "Set to truthy to force enable verification (overrides NO_VERIFY)"},
        {"NO_VERIFY", "Set to truthy to disable verification"},
        {"LOG_LEVEL", "Structured log level for file output (trace|debug|info|warn|error|fatal|panic)"},
        {"LOG_FILE", "Path to write structured JSON logs (default goresearch.log)"},
    }
    for _, e := range envs {
        b.WriteString(fmt.Sprintf("- `%s`: %s\n", e.key, e.desc))
    }

    b.WriteString("\nGenerated by `goresearch doc`.\n")
    _ = reflect.TypeOf(0) // keep reflect import if env source expands later
    return b.String()
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
        sslVerify       bool
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
        verifyEnabled      bool
        noVerify           bool
        reportsDir         string
        reportsTar         bool
        // Logging flags
        logLevel           string
        logFile            string
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
    fs.BoolVar(&sslVerify, "ssl.verify", getenv("SSL_VERIFY") != "false", "Enable SSL certificate verification (set to false for self-signed certs)")
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
    // Verification toggle flags
    fs.BoolVar(&verifyEnabled, "verify", true, "Enable the fact-check verification pass and Evidence check appendix")
    fs.BoolVar(&noVerify, "no-verify", false, "Disable the fact-check verification pass and Evidence check appendix")
    // Tools orchestration flags (config flags item)
    fs.BoolVar(&toolsEnabled, "tools.enable", false, "Enable tool-orchestrated chat mode")
    fs.BoolVar(&toolsDryRun, "tools.dryRun", false, "Do not execute tools; emit dry-run envelopes")
    fs.IntVar(&toolsMaxCalls, "tools.maxCalls", 32, "Max tool calls per run")
    fs.DurationVar(&toolsMaxWallClock, "tools.maxWallClock", 0, "Max wall-clock duration for tool loop (e.g. 30s); 0 disables")
    fs.DurationVar(&toolsPerToolTimeout, "tools.perToolTimeout", 10*time.Second, "Per-tool execution timeout (e.g. 10s)")
    fs.StringVar(&toolsMode, "tools.mode", "harmony", "Chat protocol mode: harmony|legacy")
    // Artifacts bundle flags
    fs.StringVar(&reportsDir, "reports.dir", "reports", "Root directory to persist artifacts bundles (reports)")
    fs.BoolVar(&reportsTar, "reports.tar", false, "Also produce a tar.gz of the bundle with digests for offline audit")
    // Logging flags
    fs.StringVar(&logLevel, "log.level", strings.TrimSpace(getenv("LOG_LEVEL")), "Structured log level for file output: trace|debug|info|warn|error|fatal|panic (default info)")
    fs.StringVar(&logFile, "log.file", strings.TrimSpace(getenv("LOG_FILE")), "Path to write structured JSON logs (default goresearch.log)")

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
        SSLVerify:       sslVerify,
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
        ReportsDir:      reportsDir,
        ReportsTar:      reportsTar,
        LogLevel:        logLevel,
        LogFilePath:     logFile,
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
    // Apply verification toggle precedence:
    // - if --no-verify set, disable
    // - else if --verify explicitly false (rare), disable
    // - else enabled (default)
    if noVerify {
        cfg.DisableVerify = true
    } else if !verifyEnabled {
        cfg.DisableVerify = true
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
