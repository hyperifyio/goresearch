package app

import (
	"context"
	"fmt"
	"os"
	"time"
    "strings"

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
	cfg     Config
	ai      *openai.Client
	planner PlannerFacade
    httpCache *cache.HTTPCache
}

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
        a.httpCache = &cache.HTTPCache{Dir: cfg.CacheDir}
    }

	// Quick connectivity check to local LLM by listing models
	// Do not fail hard if unreachable in dry-run; warn instead.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	models, err := a.ai.ListModels(ctx)
	if err != nil {
		if cfg.DryRun {
			log.Warn().Err(err).Msg("LLM model list failed; continuing due to dry-run")
		} else {
			return nil, fmt.Errorf("list models: %w", err)
		}
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
			selected = sel.Select(merged, sel.Options{MaxTotal: a.cfg.MaxSources, PerDomain: a.cfg.PerDomainCap, MinSnippetChars: a.cfg.MinSnippetChars})
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
        selected = sel.Select(merged, sel.Options{MaxTotal: a.cfg.MaxSources, PerDomain: a.cfg.PerDomainCap, MinSnippetChars: a.cfg.MinSnippetChars})
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
    }}
    excerpts := make([]synth.SourceExcerpt, 0, len(selected))
    for i, r := range selected {
        body, _, err := f.get(ctx, r.URL)
        if err != nil {
            log.Warn().Err(err).Str("url", r.URL).Msg("fetch failed")
            continue
        }
        doc := extract.FromHTML(body)
        // Enforce per-source excerpt size cap
        capChars := a.cfg.PerSourceChars
        if capChars <= 0 {
            capChars = 12_000
        }
        text := doc.Text
        if len(text) > capChars {
            text = text[:capChars]
        }
        excerpts = append(excerpts, synth.SourceExcerpt{Index: i + 1, Title: pickNonEmpty(doc.Title, r.Title), URL: r.URL, Excerpt: text})
    }
    // Proportionally truncate excerpts to fit global context budget while preserving all sources
    excerpts = proportionallyTruncateExcerpts(b, plan.Outline, excerpts, a.cfg)

    // 5) Synthesize report
    syn := &synth.Synthesizer{Client: a.ai, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir}, Verbose: a.cfg.Verbose}
    md, err := syn.Synthesize(ctx, synth.Input{
        Brief:        b,
        Outline:      plan.Outline,
        Sources:      excerpts,
        Model:        a.cfg.LLMModel,
        LanguageHint: a.cfg.LanguageHint,
        ReservedOutputTokens: a.cfg.ReservedOutputTokens,
    })
    if err != nil {
        return fmt.Errorf("synthesize: %w", err)
    }

    // 6) Validate citations and references. If invalid, keep document but append a warning.
    if err := validate.ValidateReport(md); err != nil {
        log.Warn().Err(err).Msg("report validation issues")
        md += "\n\n> WARNING: Validation noted issues: " + err.Error() + "\n"
    }

    // 7) Verification pass: extract claims and append an evidence map appendix.
    verifier := &verify.Verifier{Client: a.ai, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir}}
    vres, verr := verifier.Verify(ctx, md, a.cfg.LLMModel, a.cfg.LanguageHint)
    evidence := "\n\n## Evidence check\n\n"
    if verr != nil {
        log.Warn().Err(verr).Msg("verification failed; continuing without appendix")
        evidence += "> Verification failed; main report preserved.\n"
    } else {
        evidence += vres.Summary + "\n\n"
        for i, c := range vres.Claims {
            if i >= 20 { // safety cap for output size
                break
            }
            // Format: - Claim — cites [1,2]; confidence: high; supported: true
            evidence += fmt.Sprintf("- %s — cites %v; confidence: %s; supported: %t\n", c.Text, c.Citations, c.Confidence, c.Supported)
        }
    }
    md += evidence

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
