package template

import "strings"

// Type represents the supported report types
type Type string

const (
	// IMRaD follows Introduction, Methods, Results, and Discussion structure
	IMRaD Type = "imrad"
	// Decision follows a technical decision report structure
	Decision Type = "decision"
	// Literature follows literature review structure
	Literature Type = "literature"
	// Default represents the standard general report structure
	Default Type = ""
)

// Profile defines the structure and requirements for a specific report type
type Profile struct {
	Type           Type
	Name           string
	Description    string
	Outline        []string
	SystemPrompt   string
	UserPromptHint string
}

// GetProfile returns the appropriate profile for the given report type
func GetProfile(reportType string) Profile {
	t := Type(normalizeType(reportType))
	
	switch t {
	case IMRaD:
		return imradProfile()
	case Decision:
		return decisionProfile()
	case Literature:
		return literatureProfile()
	default:
		return defaultProfile()
	}
}

// normalizeType converts string input to canonical Type value
func normalizeType(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "imrad", "i.m.r.a.d", "i m r a d", "introduction, methods, results, discussion":
		return string(IMRaD)
	case "decision", "decision report", "tech", "technical", "technical report", 
		 "technical decision", "decision/tech", "decision tech":
		return string(Decision)
	case "literature", "literature review", "lit review", "systematic review", "review":
		return string(Literature)
	default:
		// Try to map substrings conservatively
		if strings.Contains(v, "imrad") {
			return string(IMRaD)
		}
		if strings.Contains(v, "decision") || strings.Contains(v, "technical") || strings.Contains(v, "tech") {
			return string(Decision)
		}
		if strings.Contains(v, "review") || strings.Contains(v, "literature") {
			return string(Literature)
		}
		return string(Default)
	}
}

// imradProfile returns the Introduction, Methods, Results, and Discussion profile
func imradProfile() Profile {
	return Profile{
		Type:        IMRaD,
		Name:        "IMRaD Report",
		Description: "Introduction, Methods, Results, and Discussion scientific report structure",
		Outline: []string{
			"Executive summary",
			"Introduction",
			"Methods",
			"Results", 
			"Discussion",
			"Alternatives & conflicting evidence",
			"Risks and limitations",
			"References",
		},
		SystemPrompt: "You are a scientific technical writer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Follow IMRaD structure: Introduction establishes context and objectives, Methods describes approach and methodology, Results presents findings objectively, Discussion interprets implications and significance. Keep style precise, objective, and scholarly.",
		UserPromptHint: "Follow IMRaD structure: Introduction (context/objectives), Methods (approach), Results (findings), Discussion (interpretation/implications).",
	}
}

// decisionProfile returns the technical decision report profile
func decisionProfile() Profile {
	return Profile{
		Type:        Decision,
		Name:        "Technical Decision Report",
		Description: "Technical decision documentation with problem, options, criteria, and recommendation",
		Outline: []string{
			"Executive summary",
			"Problem statement",
			"Decision criteria",
			"Options evaluated",
			"Recommendation",
			"Implementation considerations",
			"Alternatives & conflicting evidence",
			"Risks and limitations",
			"References",
		},
		SystemPrompt: "You are a technical decision writer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Structure as a decision document: clearly state the problem, establish evaluation criteria, analyze options objectively, make a clear recommendation with rationale, and address implementation concerns. Keep style concise, actionable, and decision-focused.",
		UserPromptHint: "Structure as decision document: Problem statement, Decision criteria, Options evaluated with pros/cons, Clear recommendation with rationale, Implementation considerations.",
	}
}

// literatureProfile returns the literature review profile
func literatureProfile() Profile {
	return Profile{
		Type:        Literature,
		Name:        "Literature Review",
		Description: "Systematic review and synthesis of existing literature on a topic",
		Outline: []string{
			"Executive summary",
			"Background and scope",
			"Review methodology", 
			"Thematic analysis",
			"Key findings synthesis",
			"Research gaps identified",
			"Alternatives & conflicting evidence",
			"Risks and limitations",
			"References",
		},
		SystemPrompt: "You are an academic literature reviewer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Structure as a literature review: establish scope and methodology, synthesize findings thematically, identify patterns and gaps in the literature, analyze conflicting viewpoints objectively. Keep style scholarly, analytical, and synthesis-focused.",
		UserPromptHint: "Structure as literature review: Background/scope, Review methodology, Thematic synthesis of findings, Identification of research gaps and patterns.",
	}
}

// defaultProfile returns the standard general report profile
func defaultProfile() Profile {
	return Profile{
		Type:        Default,
		Name:        "General Report",
		Description: "Standard general-purpose report structure",
		Outline: []string{
			"Executive summary",
			"Background",
			"Core concepts",
			"Implementation guidance",
			"Examples",
			"Alternatives & conflicting evidence",
			"Risks and limitations", 
			"References",
		},
		SystemPrompt: "You are a careful technical writer. Use ONLY the provided sources for facts. Cite precisely with bracketed numeric indices like [1] that map to the numbered references list. Do not invent sources or content. Keep style concise and factual.",
		UserPromptHint: "",
	}
}