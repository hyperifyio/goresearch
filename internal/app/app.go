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
	"github.com/hyperifyio/goresearch/internal/robots"
	"github.com/hyperifyio/goresearch/internal/planner"
	"github.com/hyperifyio/goresearch/internal/search"
	sel "github.com/hyperifyio/goresearch/internal/select"
	"github.com/hyperifyio/goresearch/internal/synth"
	"github.com/hyperifyio/goresearch/internal/validate"
	"github.com/hyperifyio/goresearch/internal/verify"
)

// extractRobotsDetails attempts to pull structured details out of robots-related
// errors emitted by the fetcher so they can be logged and captured in the
// manifest's skipped section.
func extractRobotsDetails(err error) (host, agent, directive, pattern string) {
    type det interface{ RobotsDetails() (string, string, string, string) }
    if d, ok := err.(det); ok {
        return d.RobotsDetails()
    }
    return "", "", "", ""
}

func formatRobotsDetails(host, agent, directive, pattern string) string {
    var b strings.Builder
    b.WriteString(" (")
    wrote := false
    if strings.TrimSpace(host) != "" {
        b.WriteString("host=")
        b.WriteString(strings.TrimSpace(host))
        wrote = true
    }
    if strings.TrimSpace(agent) != "" {
        if wrote { b.WriteString(" ") }
        b.WriteString("ua=")
        b.WriteString(strings.TrimSpace(agent))
        wrote = true
    }
    if strings.TrimSpace(directive) != "" {
        if wrote { b.WriteString(" ") }
        b.WriteString("dir=")
        b.WriteString(strings.TrimSpace(directive))
        wrote = true
    }
    if strings.TrimSpace(pattern) != "" {
        if wrote { b.WriteString(" ") }
        b.WriteString("pattern=")
        b.WriteString(strings.TrimSpace(pattern))
        wrote = true
    }
    if !wrote {
        return ""
    }
    b.WriteString(")")
    return b.String()
}

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
        a.httpCache = &cache.HTTPCache{Dir: cfg.CacheDir, StrictPerms: cfg.CacheStrictPerms}
	}

    // Quick connectivity check to local LLM by listing models. Skip when
    // operating in cache-only mode to avoid any network attempts.
    if !cfg.LLMCacheOnly {
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
    }

	return a, nil
}

func (a *App) Close() {
	// nothing yet
}

func (a *App) Run(ctx context.Context) error {
    if a.cfg.DryRun {
        // Minimal dry-run output including parsed brief to satisfy transparency.
        stageStart := time.Now()
        inputBytes, _ := os.ReadFile(a.cfg.InputPath)
        b := brief.ParseBrief(string(inputBytes))
        log.Info().Str("stage", "brief").Str("topic", b.Topic).Dur("elapsed", time.Since(stageStart)).Msg("brief parsed")
        // Plan queries and select URLs without calling the synthesizer
        stageStart = time.Now()
        plan := a.planQueries(ctx, b)
        log.Info().Str("stage", "planner").Int("queries", len(plan.Queries)).Strs("queries", plan.Queries).Dur("elapsed", time.Since(stageStart)).Msg("planner completed")
        // Fake search with zero provider if not configured
    var provider search.Provider
    if a.cfg.FileSearchPath != "" {
        provider = &search.FileProvider{Path: a.cfg.FileSearchPath, Policy: search.DomainPolicy{Allowlist: a.cfg.DomainAllowlist, Denylist: a.cfg.DomainDenylist}}
    } else if a.cfg.SearxURL != "" {
        provider = &search.SearxNG{BaseURL: a.cfg.SearxURL, APIKey: a.cfg.SearxKey, HTTPClient: newHighThroughputHTTPClient(), UserAgent: "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)", Policy: search.DomainPolicy{Allowlist: a.cfg.DomainAllowlist, Denylist: a.cfg.DomainDenylist}}
    }
		var selected []search.Result
		if provider != nil {
            stageStart = time.Now()
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
            urls := make([]string, 0, len(selected))
            for _, r := range selected { urls = append(urls, r.URL) }
            log.Info().Str("stage", "selection").Int("selected", len(selected)).Strs("urls", urls).Dur("elapsed", time.Since(stageStart)).Msg("search+selection completed")
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
    stageStart := time.Now()
	inputBytes, err := os.ReadFile(a.cfg.InputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	b := brief.ParseBrief(string(inputBytes))
    log.Info().Str("stage", "brief").Str("topic", b.Topic).Dur("elapsed", time.Since(stageStart)).Msg("brief parsed")

	// 2) Plan queries (LLM first with fallback)
    stageStart = time.Now()
	plan := a.planQueries(ctx, b)
    log.Info().Str("stage", "planner").Int("queries", len(plan.Queries)).Strs("queries", plan.Queries).Dur("elapsed", time.Since(stageStart)).Msg("planner completed")

    // 3) Perform searches and aggregate
    stageStart = time.Now()
    var provider search.Provider
    // Support file-based provider for deterministic/local runs (parity with dry-run)
    if a.cfg.FileSearchPath != "" {
        provider = &search.FileProvider{Path: a.cfg.FileSearchPath}
    } else if a.cfg.SearxURL != "" {
        ua := a.cfg.SearxUA
        if strings.TrimSpace(ua) == "" {
            ua = "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)"
        }
        provider = &search.SearxNG{BaseURL: a.cfg.SearxURL, APIKey: a.cfg.SearxKey, HTTPClient: newHighThroughputHTTPClient(), UserAgent: ua, Policy: search.DomainPolicy{Allowlist: a.cfg.DomainAllowlist, Denylist: a.cfg.DomainDenylist}}
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
    // Log selected URLs for traceability
    if len(selected) > 0 {
        urls := make([]string, 0, len(selected))
        for _, r := range selected {
            urls = append(urls, r.URL)
        }
        log.Info().Str("stage", "selection").Int("selected", len(selected)).Strs("urls", urls).Dur("elapsed", time.Since(stageStart)).Msg("search+selection completed")
    } else {
        log.Info().Str("stage", "selection").Int("selected", 0).Dur("elapsed", time.Since(stageStart)).Msg("search+selection completed")
    }

	// 4) Fetch and extract content for each selected URL with polite settings
    stageStart = time.Now()
	httpClient := newHighThroughputHTTPClient()
    // Configure robots manager for crawl-delay and polite fetching
    rb := &robots.Manager{HTTPClient: httpClient, Cache: a.httpCache, UserAgent: "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)", EntryExpiry: 30 * time.Minute, AllowPrivateHosts: a.cfg.AllowPrivateHosts, OverrideAllowlist: a.cfg.RobotsOverrideAllowlist, OverrideConfirm: a.cfg.RobotsOverrideConfirm}
    f := &fetchClient{client: &fetch.Client{
		HTTPClient:        httpClient,
		UserAgent:         "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)",
		MaxAttempts:       2,
		PerRequestTimeout: 15 * time.Second,
		Cache:             a.httpCache,
		RedirectMaxHops:   5,
		MaxConcurrent:     8,
		BypassCache:       a.cfg.CacheMaxAge == 0 && a.cfg.CacheClear, // bypass when user forces clear
        AllowPrivateHosts: a.cfg.AllowPrivateHosts,
        EnablePDF:         a.cfg.EnablePDF,
        Robots:            rb,
        DomainAllowlist:   a.cfg.DomainAllowlist,
        DomainDenylist:    a.cfg.DomainDenylist,
    }, cacheOnly: a.cfg.HTTPCacheOnly, httpCache: a.httpCache}
    // Use adapter-based extractor to enable swap of readability tactics
    excerpts, skipped := fetchAndExtract(ctx, f, extract.HeuristicExtractor{}, selected, a.cfg)
	// Proportionally truncate excerpts to fit global context budget while preserving all sources
	excerpts = proportionallyTruncateExcerpts(b, plan.Outline, excerpts, a.cfg)
    log.Info().Str("stage", "extract").Int("excerpts", len(excerpts)).Dur("elapsed", time.Since(stageStart)).Msg("fetch+extract completed")

    // Exit nonzero per policy when we have no usable sources.
    if len(excerpts) == 0 {
        log.Warn().Msg("no usable sources after selection and extraction")
        return ErrNoUsableSources
    }

	// 5) Synthesize report
    stageStart = time.Now()
    syn := &synth.Synthesizer{Client: a.ai, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir, StrictPerms: a.cfg.CacheStrictPerms}, Verbose: a.cfg.Verbose, SystemPrompt: a.cfg.SynthSystemPrompt, AllowCOTLogging: a.cfg.DebugVerbose, CacheOnly: a.cfg.LLMCacheOnly}
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
    log.Info().Str("stage", "synth").Int("chars", len(md)).Dur("elapsed", time.Since(stageStart)).Msg("synthesis completed")

	// 6) Validate structure and citations. If invalid, keep document but append a warning.
    stageStart = time.Now()
	if err := validate.ValidateStructure(md, plan.Outline); err != nil {
		log.Warn().Err(err).Msg("report structure issues")
		md += "\n\n> WARNING: Structure issues: " + err.Error() + "\n"
	}
	if err := validate.ValidateReport(md); err != nil {
		log.Warn().Err(err).Msg("report validation issues")
		md += "\n\n> WARNING: Validation noted issues: " + err.Error() + "\n"
	}
    // Visuals QA (figures/tables)
    if err := validate.ValidateVisuals(md); err != nil {
        log.Warn().Err(err).Msg("visuals QA issues")
        md += "\n\n> WARNING: Visuals QA issues: " + err.Error() + "\n"
    }
    // Title quality check — enforce word count, keywords, and acronym definitions
    if err := validate.ValidateTitleQuality(md); err != nil {
        log.Warn().Err(err).Msg("title quality issues")
        md += "\n\n> WARNING: Title quality issues: " + err.Error() + "\n"
    }
    // Audience fit check — flag jargon or mismatched sections vs audience/tone
    if err := validate.ValidateAudienceFit(md, b.AudienceHint, b.ToneHint); err != nil {
        log.Warn().Err(err).Msg("audience fit issues")
        md += "\n\n> WARNING: Audience fit issues: " + err.Error() + "\n"
    }
    // Distribution readiness (opt-in): ensure metadata and anchors are valid.
    if a.cfg.DistributionChecks {
        distVersion := a.cfg.ExpectedVersion
        if strings.TrimSpace(distVersion) == "" {
            distVersion = BuildVersion
        }
        if strings.TrimSpace(distVersion) == "" { distVersion = "0.0.0-dev" }
        if err := validate.ValidateDistributionReady(md, a.cfg.ExpectedAuthor, distVersion); err != nil {
            log.Warn().Err(err).Msg("distribution readiness issues")
            md += "\n\n> WARNING: Distribution readiness: " + err.Error() + "\n"
        }
    }
    log.Info().Str("stage", "validate").Dur("elapsed", time.Since(stageStart)).Msg("validation completed")

	// 7) Verification pass: extract claims and append an evidence map appendix.
    stageStart = time.Now()
    verifier := &verify.Verifier{Client: a.ai, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir, StrictPerms: a.cfg.CacheStrictPerms}, SystemPrompt: a.cfg.VerifySystemPrompt, CacheOnly: a.cfg.LLMCacheOnly}
	vres, verr := verifier.Verify(ctx, md, a.cfg.LLMModel, a.cfg.LanguageHint)
	if verr != nil {
		log.Warn().Err(verr).Msg("verification failed; continuing without appendix")
	}
	md = appendEvidenceAppendix(md, vres, verr)
    log.Info().Str("stage", "verify").Bool("ok", verr == nil).Dur("elapsed", time.Since(stageStart)).Msg("verification completed")

    // 7b) Glossary & acronym list — auto-extract key terms and append optional appendix
    md = appendGlossaryAppendix(md)

	// 8) Append reproducibility footer capturing model/base URL, source count, and cache status
	md = appendReproFooter(md, a.cfg.LLMModel, a.cfg.LLMBaseURL, len(excerpts), a.httpCache != nil, true)

    // 9) Append embedded manifest and write a sidecar JSON manifest
    stageStart = time.Now()
	manEntries := buildManifestEntriesFromSynth(excerpts)
	manMeta := manifestMeta{
		Model:       a.cfg.LLMModel,
		LLMBaseURL:  a.cfg.LLMBaseURL,
		SourceCount: len(excerpts),
		HTTPCache:   a.httpCache != nil,
		LLMCache:    true,
		GeneratedAt: time.Now().UTC(),
	}
    // Include a list of skipped URLs due to robots/opt-out decisions in the manifest
    md = appendEmbeddedManifestWithSkipped(md, manMeta, manEntries, skipped)
    // If tools were used this run and a transcript exists, append it
    if a.cfg.ToolsEnabled {
        // The orchestrated path would own synthesis; for baseline pipeline we have no transcript.
        // We append only when orchestrator provides one in future integration.
        // noop for now
    }
	if data, err := marshalManifestJSON(manMeta, manEntries); err == nil {
		_ = os.WriteFile(deriveManifestSidecarPath(a.cfg.OutputPath), data, 0o644)
	}
    log.Info().Str("stage", "manifest").Int("sources", len(manEntries)).Dur("elapsed", time.Since(stageStart)).Msg("manifest written")

    // 10) Optionally render a PDF copy with basic clickable links
    if strings.TrimSpace(a.cfg.OutputPDFPath) != "" {
        if err := writeSimplePDF(md, a.cfg.OutputPDFPath); err != nil {
            log.Warn().Err(err).Str("pdf", a.cfg.OutputPDFPath).Msg("failed to write PDF copy")
        } else {
            log.Info().Str("pdf", a.cfg.OutputPDFPath).Msg("wrote PDF copy")
        }
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
        a.planner.llm = &planner.LLMPlanner{Client: a.ai, Model: a.cfg.LLMModel, LanguageHint: a.cfg.LanguageHint, Cache: &cache.LLMCache{Dir: a.cfg.CacheDir, StrictPerms: a.cfg.CacheStrictPerms}, CacheOnly: a.cfg.LLMCacheOnly}
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
    cacheOnly bool
    httpCache *cache.HTTPCache
}

func (f *fetchClient) get(ctx context.Context, url string) ([]byte, string, error) {
    if f == nil {
        return nil, "", fmt.Errorf("fetch client not configured")
    }
    if f.cacheOnly {
        if f.httpCache == nil {
            return nil, "", fmt.Errorf("http cache-only mode but no cache configured")
        }
        // Load without network; fail fast on miss
        body, err := f.httpCache.LoadBody(ctx, url)
        if err != nil {
            return nil, "", fmt.Errorf("http cache-only: not found")
        }
        meta, err := f.httpCache.LoadMeta(ctx, url)
        if err != nil || meta == nil {
            return nil, "", fmt.Errorf("http cache-only: not found meta")
        }
        return body, meta.ContentType, nil
    }
    if f.client == nil {
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

func fetchAndExtract(ctx context.Context, f sourceGetter, extractor interface{ Extract([]byte) extract.Document }, selected []search.Result, cfg Config) ([]synth.SourceExcerpt, []skippedEntry) {
    excerpts := make([]synth.SourceExcerpt, 0, len(selected))
    skipped := make([]skippedEntry, 0)
	capChars := cfg.PerSourceChars
	if capChars <= 0 {
		capChars = 12_000
	}
	nextIndex := 1
	for _, r := range selected {
        body, contentType, err := f.get(ctx, r.URL)
		if err != nil {
            if reason, denied := fetch.IsReuseDenied(err); denied {
                log.Info().Str("url", r.URL).Str("reason", reason).Msg("skipping due to robots/opt-out")
                skipped = append(skipped, skippedEntry{URL: r.URL, Reason: reason})
                continue
            }
            if reason, denied := fetch.IsRobotsDenied(err); denied {
                // Try to enrich reason with details if available
                host, agent, directive, pattern := extractRobotsDetails(err)
                det := reason
                if directive != "" || agent != "" || pattern != "" {
                    // Format: reason (host=.. ua=.. dir=.. pattern=..)
                    det = det + formatRobotsDetails(host, agent, directive, pattern)
                }
                log.Info().Str("url", r.URL).Str("reason", det).Msg("skipping due to robots/disallow")
                skipped = append(skipped, skippedEntry{URL: r.URL, Reason: det})
                continue
            }
            log.Warn().Err(err).Str("url", r.URL).Msg("fetch failed; skipping source")
            continue
		}
        // Choose extraction strategy based on content type and config
        var doc extract.Document
        if cfg.EnablePDF && strings.HasPrefix(strings.ToLower(contentType), "application/pdf") {
            doc = extract.FromPDF(body)
        } else {
            if extractor != nil {
                doc = extractor.Extract(body)
            } else {
                doc = extract.FromHTML(body)
            }
        }
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
    return excerpts, skipped
}
