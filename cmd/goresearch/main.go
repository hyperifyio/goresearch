package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
    "github.com/rs/zerolog/log"

	"github.com/hyperifyio/goresearch/internal/app"
)

func main() {
	// Logging setup
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	var (
		inputPath      string
		outputPath     string
		searxURL       string
		searxKey       string
		llmBaseURL     string
		llmModel       string
		llmKey         string
		maxSources     int
		perDomain      int
		perSourceChars int
        minSnippetChars int
		language       string
		dryRun         bool
		verbose        bool
        cacheDir       string
        cacheMaxAge    time.Duration
        cacheClear     bool
        topicHash      string
	)

	flag.StringVar(&inputPath, "input", "request.md", "Path to input Markdown research request")
	flag.StringVar(&outputPath, "output", "report.md", "Path to write the final Markdown report")
	flag.StringVar(&searxURL, "searx.url", os.Getenv("SEARX_URL"), "SearxNG base URL")
	flag.StringVar(&searxKey, "searx.key", os.Getenv("SEARX_KEY"), "SearxNG API key (optional)")
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
	flag.StringVar(&cacheDir, "cache.dir", ".goresearch-cache", "Cache directory path")
    flag.DurationVar(&cacheMaxAge, "cache.maxAge", 0, "Max age for cache entries before purge (e.g. 24h, 7d); 0 disables")
    flag.BoolVar(&cacheClear, "cache.clear", false, "Clear cache directory before run")
    flag.StringVar(&topicHash, "cache.topicHash", os.Getenv("TOPIC_HASH"), "Optional topic hash to scope cache; accepted for traceability")
    flag.Parse()

	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	cfg := app.Config{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		SearxURL:       searxURL,
		SearxKey:       searxKey,
		LLMBaseURL:     llmBaseURL,
		LLMModel:       llmModel,
		LLMAPIKey:      llmKey,
		MaxSources:     maxSources,
		PerDomainCap:   perDomain,
		PerSourceChars: perSourceChars,
        MinSnippetChars: minSnippetChars,
		LanguageHint:   language,
		DryRun:         dryRun,
		CacheDir:       cacheDir,
        Verbose:        verbose,
        CacheMaxAge:    cacheMaxAge,
        CacheClear:     cacheClear,
        TopicHash:      topicHash,
	}

	if err := run(cfg); err != nil {
		log.Error().Err(err).Msg("run failed")
		os.Exit(1)
	}
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
