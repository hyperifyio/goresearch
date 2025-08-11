package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/hyperifyio/goresearch/internal/brief"
	"github.com/hyperifyio/goresearch/internal/cache"
    "github.com/hyperifyio/goresearch/internal/llm"
	"github.com/hyperifyio/goresearch/internal/template"
	"github.com/rs/zerolog/log"
)

// Plan represents the structured result from the planner step.
type Plan struct {
	Queries []string `json:"queries"`
	Outline []string `json:"outline"`
}

// Planner produces web search queries and a section outline from a Brief.
type Planner interface {
	Plan(ctx context.Context, b brief.Brief) (Plan, error)
}

// LLMPlanner calls an OpenAI-compatible endpoint and enforces a JSON-only contract.
type LLMPlanner struct {
    Client       llm.Client
	Model        string
	LanguageHint string
	Cache        *cache.LLMCache
	Verbose      bool
    // CacheOnly, when true, returns from cache and fails fast if missing.
    CacheOnly    bool
}

func buildSystemMessage(b brief.Brief) string {
	profile := template.GetProfile(b.ReportType)
	base := "You are a planning assistant. Respond with strict JSON only, no narration. The JSON schema is {\"queries\": string[6..10], \"outline\": string[5..8]}. Queries must be diverse and concise, and MUST include at least two that explicitly seek counter-evidence or alternatives, e.g., 'limitations of <topic>', 'contrary findings about <topic>', or 'alternatives to <topic>'. The outline must contain a heading 'Alternatives & conflicting evidence'. Outline contains section headings only."
	
	if profile.Type != template.Default {
		base += fmt.Sprintf(" For %s reports, prefer section headings that follow the %s structure.", profile.Name, strings.ToLower(string(profile.Type)))
	}
	
	return base
}

// Plan implements Planner using the chat completions API. If the model returns
// non-JSON or the payload cannot be parsed, an error is returned so callers can
// choose to fall back.
func (p *LLMPlanner) Plan(ctx context.Context, b brief.Brief) (Plan, error) {
	if p.Client == nil || p.Model == "" {
		return Plan{}, errors.New("planner not configured")
	}

	user := buildUserPrompt(b, p.LanguageHint)
	system := buildSystemMessage(b)
    // Cache lookup
    if p.Cache != nil {
		key := cache.KeyFrom(p.Model, system+"\n\n"+user)
		if raw, ok, _ := p.Cache.Get(ctx, key); ok {
			var plan Plan
			if err := json.Unmarshal(raw, &plan); err == nil {
				return plan, nil
			}
		}
	}
    if p.CacheOnly {
        return Plan{}, errors.New("planner cache-only: not found")
    }
    if p.Verbose {
        // Log prompt skeleton only; avoid logging raw excerpts or sensitive data
        log.Debug().Str("stage", "planner").Str("model", p.Model).Int("system_len", len(system)).Int("user_len", len(user)).Msg("planner prompt")
    }
	resp, err := p.Client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.1,
		N:           1,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("planner call: %w", err)
	}
    if len(resp.Choices) == 0 {
		return Plan{}, errors.New("no choices")
	}
	var plan Plan
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return Plan{}, fmt.Errorf("parse planner json: %w", err)
	}
    	plan.Queries = ensureCounterEvidenceQueries(b.Topic, sanitizeQueries(plan.Queries), p.LanguageHint)
    plan.Outline = mergeTemplateOutline(b.ReportType, sanitizeOutline(plan.Outline))
	if len(plan.Queries) < 3 || len(plan.Outline) < 3 {
		return Plan{}, errors.New("insufficient planner output")
	}
	if p.Cache != nil {
		if b, err := json.Marshal(plan); err == nil {
			_ = p.Cache.Save(ctx, cache.KeyFrom(p.Model, system+"\n\n"+user), b)
		}
	}
	return plan, nil
}

// FallbackPlanner produces deterministic queries and a generic outline when the
// LLM planner is unavailable or returns invalid output.
type FallbackPlanner struct {
	LanguageHint string
}

func (p *FallbackPlanner) Plan(_ context.Context, b brief.Brief) (Plan, error) {
	topic := strings.TrimSpace(b.Topic)
	if topic == "" {
		topic = "research topic"
	}
    // Deterministic set of queries including counter-evidence/alternatives
    words := []string{"specification", "documentation", "reference", "tutorial", "best practices", "faq", "examples", "comparison", "limitations", "contrary findings", "alternatives"}
    queries := make([]string, 0, 10)
	for _, w := range words {
		q := topic + " " + w
		if p.LanguageHint != "" {
			q = q + " (" + p.LanguageHint + ")"
		}
        queries = append(queries, q)
        if len(queries) == 10 { // cap to schema range
            break
        }
	}
    // Use template system for outline
    profile := template.GetProfile(b.ReportType)
    outline := profile.Outline
    return Plan{Queries: queries, Outline: outline}, nil
}

func buildUserPrompt(b brief.Brief, lang string) string {
	var sb strings.Builder
	sb.WriteString("Brief topic: ")
	sb.WriteString(b.Topic)
	if b.AudienceHint != "" {
		sb.WriteString("\nAudience: ")
		sb.WriteString(b.AudienceHint)
	}
	if b.ToneHint != "" {
		sb.WriteString("\nTone: ")
		sb.WriteString(b.ToneHint)
	}
	if b.TargetLengthWords > 0 {
		sb.WriteString("\nTarget length: ")
		sb.WriteString(fmt.Sprintf("%d words", b.TargetLengthWords))
	}
	if lang != "" {
		sb.WriteString("\nLanguage: ")
		sb.WriteString(lang)
	}
	return sb.String()
}

func sanitizeQueries(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, q := range in {
		s := strings.TrimSpace(q)
		if s == "" {
			continue
		}
		s = strings.TrimSuffix(s, ".")
		s = strings.TrimSuffix(s, "?")
		s = strings.TrimSpace(s)
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func sanitizeOutline(in []string) []string {
	out := make([]string, 0, len(in))
	for _, h := range in {
		s := strings.TrimSpace(h)
		s = strings.Trim(s, "# ")
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// ensureCounterEvidenceQueries appends counter-evidence/alternatives queries
// when missing, capping the list at 10 entries.
func ensureCounterEvidenceQueries(topic string, in []string, lang string) []string {
    have := map[string]bool{}
    out := make([]string, 0, len(in))
    for _, q := range in {
        out = append(out, q)
        have[strings.ToLower(strings.TrimSpace(q))] = true
    }
    mk := func(suffix string) string {
        q := strings.TrimSpace(topic + " " + suffix)
        if strings.TrimSpace(lang) != "" {
            q += " (" + lang + ")"
        }
        return q
    }
    candidates := []string{
        mk("limitations"),
        mk("contrary findings"),
        mk("alternatives"),
        mk("criticisms"),
    }
    for _, c := range candidates {
        if len(out) >= 10 {
            break
        }
        key := strings.ToLower(strings.TrimSpace(c))
        if !have[key] {
            out = append(out, c)
            have[key] = true
        }
    }
    if len(out) > 10 {
        out = out[:10]
    }
    return out
}

// mergeTemplateOutline merges LLM-generated outline with template requirements.
// Uses the template's outline as base and incorporates unique sections from LLM output.
func mergeTemplateOutline(reportType string, llmOutline []string) []string {
    profile := template.GetProfile(reportType)
    templateOutline := profile.Outline
    
    // If no LLM outline or template outline is empty, return template outline
    if len(llmOutline) == 0 {
        return templateOutline
    }
    if len(templateOutline) == 0 {
        return ensureRequiredSections(llmOutline)
    }
    
    // Create a map of normalized template sections
    templateSections := make(map[string]string)
    for _, section := range templateOutline {
        normalized := strings.ToLower(strings.TrimSpace(section))
        templateSections[normalized] = section
    }
    
    // Start with template outline as base
    result := make([]string, 0, len(templateOutline))
    result = append(result, templateOutline...)
    
    // Add unique sections from LLM outline that don't conflict with template
    for _, llmSection := range llmOutline {
        normalized := strings.ToLower(strings.TrimSpace(llmSection))
        if _, exists := templateSections[normalized]; !exists {
            // Insert unique LLM section before "Alternatives & conflicting evidence" if possible
            inserted := false
            for i, section := range result {
                if strings.EqualFold(strings.TrimSpace(section), "Alternatives & conflicting evidence") {
                    // Insert before this section
                    result = append(result[:i], append([]string{llmSection}, result[i:]...)...)
                    inserted = true
                    break
                }
            }
            if !inserted {
                // Insert before "Risks and limitations" as fallback
                for i, section := range result {
                    if strings.EqualFold(strings.TrimSpace(section), "Risks and limitations") {
                        result = append(result[:i], append([]string{llmSection}, result[i:]...)...)
                        inserted = true
                        break
                    }
                }
            }
            if !inserted {
                // Insert before "References" as last resort
                for i, section := range result {
                    if strings.EqualFold(strings.TrimSpace(section), "References") {
                        result = append(result[:i], append([]string{llmSection}, result[i:]...)...)
                        inserted = true
                        break
                    }
                }
            }
        }
    }
    
    return result
}

// ensureRequiredSections ensures the outline contains required sections
func ensureRequiredSections(outline []string) []string {
    required := []string{
        "Alternatives & conflicting evidence",
        "Risks and limitations", 
        "References",
    }
    
    result := make([]string, len(outline))
    copy(result, outline)
    
    for _, req := range required {
        found := false
        for _, section := range result {
            if strings.EqualFold(strings.TrimSpace(section), req) {
                found = true
                break
            }
        }
        if !found {
            result = append(result, req)
        }
    }
    
    return result
}
