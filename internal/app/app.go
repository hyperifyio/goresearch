package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"

	"github.com/hyperifyio/goresearch/internal/aggregate"
	"github.com/hyperifyio/goresearch/internal/brief"
	"github.com/hyperifyio/goresearch/internal/cache"
	"github.com/hyperifyio/goresearch/internal/extract"
	"github.com/hyperifyio/goresearch/internal/fetch"
	"github.com/hyperifyio/goresearch/internal/planner"
	"github.com/hyperifyio/goresearch/internal/search"
	sel "github.com/hyperifyio/goresearch/internal/select"
	"github.com/hyperifyio/goresearch/internal/synth"
	"github.com/hyperifyio/goresearch/internal/validate"
	"github.com/hyperifyio/goresearch/internal/verify"
)

type App struct {
	cfg       Config
	ai        *openai.Client
	planner   PlannerFacade
	httpCache *cache.HTTPCache
}

// ErrNoUsableSources is returned when the pipeline ends up with zero usable
// source excerpts after selection and extraction. Per the Exit code policy,
// this condition should result in a non-zero process exit.
var ErrNoUsableSources = fmt.Errorf("no usable sources")

func New(ctx context.Context, cfg Config) (*App, error) {
	// Build OpenAI-compatible config
	transportCfg := openai.DefaultConfig(cfg.LLMAPIKey)
	if cfg.LLMBaseURL != "" {
		transportCfg.BaseURL = cfg.LLMBaseURL
	}

	// Use a high-throughput HTTP client to avoid client-side throttling
	transportCfg.HTTPClient = newHighThroughputHTTPClient()
	client := openai.NewClientWithConfig(transportCfg)

	a := &App{cfg: cfg, ai: client}
	// Initialize HTTP cache lazily when needed
	if cfg.CacheDir != "" {
		// Apply cache invalidation controls
		if cfg.CacheClear {
			_ = cache.ClearDir(cfg.CacheDir)
		}
		if cfg.CacheMaxAge > 0 {
			// Purge both HTTP and LLM caches by age; ignore errors to avoid failing startup
			_, _ = cache.PurgeHTTPCacheByAge(cfg.CacheDir, cfg.CacheMaxAge)
			_, _ = cache.PurgeLLMCacheByAge(cfg.CacheDir, cfg.CacheMaxAge)
		}
		a.httpCache = &cache.HTTPCache{Dir: cfg.CacheDir}
	}

	// Quick connectivity check to local LLM by listing models
	// Do not fail hard if unreachable in dry-run; warn instead.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
    models, err := a.ai.ListModels(ctx)
    if err != nil {
        // Preflight is best-effort: do not fail hard here. We continue and let
        // downstream synthesis surface errors as needed so the CLI can apply
        // its exit code policy.
        log.Warn().Err(err).Msg("LLM model list failed; continuing")
    } else {
		if len(models.Models) > 0 {
			log.Info().Int("count", len(models.Models)).Msg("LLM models available")
		} else {
			log.Warn().Msg("LLM returned zero models")
		}
	}

	return a, nil
}

func (a *App) Close() {
	// nothing yet
}

func (a *App) Run(ctx context.Context) error {
	if a.cfg.DryRun {
		// Minimal dry-run output including parsed brief to satisfy transparency.
		inputBytes, _ := os.ReadFile(a.cfg.InputPath)
		b := brief.ParseBrief(string(inputBytes))
		// Plan queries and select URLs without calling the synthesizer
		plan := a.planQueries(ctx, b)
		// Fake search with zero provider if not configured
		var provider search.Provider
		if a.cfg.SearxURL != "" {
			provider = &search.SearxNG{BaseURL: a.cfg.SearxURL, APIKey: a.cfg.SearxKey, HTTPClient: newHighThroughputHTTPClient()}
		}
		var selected []search.Result
		if provider != nil {
			groups := make([][]search.Result, 0, len(plan.Queries))
			for _, q := range plan.Queries {
				results, err := provider.Search(ctx, q, 10)
				if err != nil {
					log.Warn().Err(err).Str("query", q).Msg("search error")
					continue
				}
				groups = append(groups, results)
			}
			merged := aggregate.MergeAndNormalize(groups)
			selected = sel.Select(merged, sel.Options{MaxTotal: a.cfg.MaxSources, PerDomain: a.cfg.PerDomainCap, MinSnippetChars: a.cfg.MinSnippetChars, PreferredLanguage: a.cfg.LanguageHint})
		}
		content := fmt.Sprintf("# goresearch (dry run)\n\nTopic: %s\nAudience: %s\nTone: %s\nTarget Length (words): %d\n\nPlanned queries:\n", b.Topic, b.AudienceHint, b.ToneHint, b.TargetLengthWords)
		for i, q := range plan.Queries {
			content += fmt.Sprintf("%d. %s\n", i+1, q)
		}
		if len(selected) > 0 {
			content += "\nSelected URLs:\n"
			for i, r := range selected {
				content += fmt.Sprintf("%d. %s — %s\n", i+1, r.Title, r.URL)
			}
		}
		// Append conservative token budget estimate for transparency
		est := estimateSynthesisBudget(b, plan.Outline, selected, a.cfg)
		content += "\nBudget estimate (synthesis):\n"
		content += fmt.Sprintf("Model: %s\n", a.cfg.LLMModel)
		content += fmt.Sprintf("Estimated prompt tokens: %d\n", est.PromptTokens)
		content += fmt.Sprintf("Reserved output tokens: %d\n", est.ReservedOutput)
		content += fmt.Sprintf("Model context window: %d\n", est.ModelContext)
		content += fmt.Sprintf("Remaining tokens: %d\n", est.Remaining)
		content += fmt.Sprintf("Fits: %t\n", est.Fits)
		// Append reproducibility footer to dry-run content as well
		content = appendReproFooter(content, a.cfg.LLMModel, a.cfg.LLMBaseURL, len(selected), a.httpCache != nil, true)
		if err := os.WriteFile(a.cfg.OutputPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		log.Info().Str("out", a.cfg.OutputPath).Msg("wrote dry-run output")
		return nil
	}

	// 1) Read and parse brief
	inputBytes, err := os.ReadFile(a.cfg.InputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	b := brief.ParseBrief(string(inputBytes))

	// 2) Plan queries (LLM first with fallback)
	plan := a.planQueries(ctx, b)

	// 3) Perform searches and aggregate
	var provider search.Provider
	if a.cfg.SearxURL != "" {
		provider = &search.SearxNG{BaseURL: a.cfg.SearxURL, APIKey: a.cfg.SearxKey, HTTPClient: newHighThroughputHTTPClient()}
	}
	var selected []search.Result
	if provider != nil {
		groups := make([][]search.Result, 0, len(plan.Queries))
		for _, q := range plan.Queries {
			results, err := provider.Search(ctx, q, 10)
			if err != nil {
				log.Warn().Err(err).Str("query", q).Msg("search error")
				continue
			}
			groups = append(groups, results)
		}
		merged := aggregate.MergeAndNormalize(groups)
		selected = sel.Select(merged, sel.Options{MaxTotal: a.cfg.MaxSources, PerDomain: a.cfg.PerDomainCap, MinSnippetChars: a.cfg.MinSnippetChars, PreferredLanguage: a.cfg.LanguageHint})
	}

	// 4) Fetch and extract content for each selected URL with polite settings
	httpClient := newHighThroughputHTTPClient()
	f := &fetchClient{client: &fetch.Client{
		HTTPClient:        httpClient,
		UserAgent:         "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)",
		MaxAttempts:       2,
		PerRequestTimeout: 15 * time.Second,
		Cache:             a.httpCache,
		RedirectMaxHops:   5,
		MaxConcurrent:     8,
		BypassCache:       a.cfg.CacheMaxAge == 0 && a.cfg.CacheClear, // bypass when user forces clear
	}}
    excerpts := fetchAndExtract(ctx, f, selected, a.cfg)
	// Proportionally truncate excerpts to fit global context budget while preserving all sources
	excerpts = proportionallyTruncateExcerpts(b, plan.Outline, excerpts, a.cfg)

    // Exit nonzero per policy when we have no usable sources.
    if len(excerpts) == 0 {
        log.Warn().Msg("no usable sources after selection and extraction")
        return ErrNoUsableSources
    }

	// 5) Synthesize report
	syn := &synth.Synthesizer{Client: a.ai, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir}, Verbose: a.cfg.Verbose}
	md, err := syn.Synthesize(ctx, synth.Input{
		Brief:                b,
		Outline:              plan.Outline,
		Sources:              excerpts,
		Model:                a.cfg.LLMModel,
		LanguageHint:         a.cfg.LanguageHint,
		ReservedOutputTokens: a.cfg.ReservedOutputTokens,
	})
	if err != nil {
		return fmt.Errorf("synthesize: %w", err)
	}

	// 6) Validate structure and citations. If invalid, keep document but append a warning.
	if err := validate.ValidateStructure(md, plan.Outline); err != nil {
		log.Warn().Err(err).Msg("report structure issues")
		md += "\n\n> WARNING: Structure issues: " + err.Error() + "\n"
	}
	if err := validate.ValidateReport(md); err != nil {
		log.Warn().Err(err).Msg("report validation issues")
		md += "\n\n> WARNING: Validation noted issues: " + err.Error() + "\n"
	}

	// 7) Verification pass: extract claims and append an evidence map appendix.
	verifier := &verify.Verifier{Client: a.ai, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir}}
	vres, verr := verifier.Verify(ctx, md, a.cfg.LLMModel, a.cfg.LanguageHint)
	if verr != nil {
		log.Warn().Err(verr).Msg("verification failed; continuing without appendix")
	}
	md = appendEvidenceAppendix(md, vres, verr)

	// 8) Append reproducibility footer capturing model/base URL, source count, and cache status
	md = appendReproFooter(md, a.cfg.LLMModel, a.cfg.LLMBaseURL, len(excerpts), a.httpCache != nil, true)

	// 9) Append embedded manifest and write a sidecar JSON manifest
	manEntries := buildManifestEntriesFromSynth(excerpts)
	manMeta := manifestMeta{
		Model:       a.cfg.LLMModel,
		LLMBaseURL:  a.cfg.LLMBaseURL,
		SourceCount: len(excerpts),
		HTTPCache:   a.httpCache != nil,
		LLMCache:    true,
		GeneratedAt: time.Now().UTC(),
	}
	md = appendEmbeddedManifest(md, manMeta, manEntries)
	if data, err := marshalManifestJSON(manMeta, manEntries); err == nil {
		_ = os.WriteFile(deriveManifestSidecarPath(a.cfg.OutputPath), data, 0o644)
	}

	if err := os.WriteFile(a.cfg.OutputPath, []byte(md), 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	log.Info().Str("out", a.cfg.OutputPath).Msg("wrote output")
	return nil
}

// PlannerFacade picks LLM planner and falls back deterministically.
type PlannerFacade struct {
	llm *planner.LLMPlanner
	fb  *planner.FallbackPlanner
}

func (a *App) planQueries(ctx context.Context, b brief.Brief) planner.Plan {
	// Build facade on first use
	if a.planner.llm == nil && a.ai != nil && a.cfg.LLMModel != "" {
		a.planner.llm = &planner.LLMPlanner{Client: a.ai, Model: a.cfg.LLMModel, LanguageHint: a.cfg.LanguageHint, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir}}
	}
	if a.planner.fb == nil {
		a.planner.fb = &planner.FallbackPlanner{LanguageHint: a.cfg.LanguageHint}
	}
	if a.planner.llm != nil {
		if p, err := a.planner.llm.Plan(ctx, b); err == nil {
			return p
		} else {
			log.Warn().Err(err).Msg("planner failed, using fallback")
		}
	}
	p, _ := a.planner.fb.Plan(ctx, b)
	return p
}

// fetchClient is a small adapter around fetch.Client to keep app package decoupled
// from the exact fetcher API shape and simplify testing.
type fetchClient struct {
	client *fetch.Client
}

func (f *fetchClient) get(ctx context.Context, url string) ([]byte, string, error) {
	if f == nil || f.client == nil {
		return nil, "", fmt.Errorf("fetch client not configured")
	}
	return f.client.Get(ctx, url)
}

func pickNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// fetchAndExtract retrieves HTML for each selected URL independently and extracts
// readable text. Errors are isolated per URL: failures are logged and skipped
// rather than aborting the whole run. This satisfies the "Per-source failure
// isolation" checklist item by ensuring one bad source does not stop progress.
// sourceGetter abstracts the minimal fetch method used for tests.
type sourceGetter interface {
	get(ctx context.Context, url string) ([]byte, string, error)
}

func fetchAndExtract(ctx context.Context, f sourceGetter, selected []search.Result, cfg Config) []synth.SourceExcerpt {
	excerpts := make([]synth.SourceExcerpt, 0, len(selected))
	capChars := cfg.PerSourceChars
	if capChars <= 0 {
		capChars = 12_000
	}
	nextIndex := 1
	for _, r := range selected {
		body, _, err := f.get(ctx, r.URL)
		if err != nil {
			log.Warn().Err(err).Str("url", r.URL).Msg("fetch failed; skipping source")
			continue
		}
		doc := extract.FromHTML(body)
		text := doc.Text
		if len(text) > capChars {
			text = text[:capChars]
		}
		excerpts = append(excerpts, synth.SourceExcerpt{
			Index:   nextIndex,
			Title:   pickNonEmpty(doc.Title, r.Title),
			URL:     r.URL,
			Excerpt: text,
		})
		nextIndex++
	}
	return excerpts
}
