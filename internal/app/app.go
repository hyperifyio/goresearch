package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"

	"github.com/hyperifyio/goresearch/internal/aggregate"
	"github.com/hyperifyio/goresearch/internal/brief"
	"github.com/hyperifyio/goresearch/internal/cache"
	"github.com/hyperifyio/goresearch/internal/planner"
	"github.com/hyperifyio/goresearch/internal/search"
	sel "github.com/hyperifyio/goresearch/internal/select"
)

type App struct {
	cfg     Config
	ai      *openai.Client
	planner PlannerFacade
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
		if err := os.WriteFile(a.cfg.OutputPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		log.Info().Str("out", a.cfg.OutputPath).Msg("wrote dry-run output")
		return nil
	}

	// TODO: implement full pipeline
	content := "# goresearch\n\nMinimal run completed. Full pipeline not yet implemented.\n"
	if err := os.WriteFile(a.cfg.OutputPath, []byte(content), 0o644); err != nil {
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
