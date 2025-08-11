package template

import (
	"context"
	"testing"

	"github.com/hyperifyio/goresearch/internal/brief"
	"github.com/hyperifyio/goresearch/internal/planner"
)

// TestTemplateIntegrationWithPlanner verifies that the template system
// integrates properly with the planner to produce appropriate outlines
func TestTemplateIntegrationWithPlanner(t *testing.T) {
	tests := []struct {
		name             string
		reportType       string
		expectedSections []string
	}{
		{
			name:       "IMRaD report",
			reportType: "imrad",
			expectedSections: []string{
				"Executive summary",
				"Introduction",
				"Methods", 
				"Results",
				"Discussion",
				"Alternatives & conflicting evidence",
				"Risks and limitations",
				"References",
			},
		},
		{
			name:       "Decision report", 
			reportType: "decision",
			expectedSections: []string{
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
		},
		{
			name:       "Literature review",
			reportType: "literature",
			expectedSections: []string{
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
		},
		{
			name:       "Default report",
			reportType: "",
			expectedSections: []string{
				"Executive summary",
				"Background",
				"Core concepts",
				"Implementation guidance",
				"Examples",
				"Alternatives & conflicting evidence",
				"Risks and limitations",
				"References",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a brief with the specified report type
			b := brief.Brief{
				Topic:      "Test topic",
				ReportType: tt.reportType,
			}

			// Use fallback planner which directly uses template system
			fallbackPlanner := &planner.FallbackPlanner{}
			plan, err := fallbackPlanner.Plan(context.Background(), b)
			if err != nil {
				t.Fatalf("FallbackPlanner.Plan() error = %v", err)
			}

			// Verify outline matches expected sections
			if len(plan.Outline) != len(tt.expectedSections) {
				t.Fatalf("got %d sections, want %d", len(plan.Outline), len(tt.expectedSections))
			}

			for i, expected := range tt.expectedSections {
				if plan.Outline[i] != expected {
					t.Errorf("section[%d] = %q, want %q", i, plan.Outline[i], expected)
				}
			}
		})
	}
}

// TestTemplateSystemPromptsAreDistinct verifies that each template
// provides meaningfully different system prompts for the synthesizer
func TestTemplateSystemPromptsAreDistinct(t *testing.T) {
	profiles := []Profile{
		GetProfile("imrad"),
		GetProfile("decision"),
		GetProfile("literature"),
		GetProfile(""), // default
	}

	// Each profile should have a unique system prompt
	prompts := make(map[string]string)
	for _, profile := range profiles {
		if existing, found := prompts[profile.SystemPrompt]; found {
			t.Errorf("Duplicate system prompt found for %s and %s", string(profile.Type), existing)
		}
		prompts[profile.SystemPrompt] = string(profile.Type)
	}

	// Verify specific keywords appear in appropriate prompts
	imradProfile := GetProfile("imrad")
	if !containsAll(imradProfile.SystemPrompt, []string{"IMRaD", "Introduction", "Methods", "Results", "Discussion"}) {
		t.Error("IMRaD profile system prompt should mention IMRaD structure components")
	}

	decisionProfile := GetProfile("decision")
	if !containsAll(decisionProfile.SystemPrompt, []string{"decision", "problem", "recommendation"}) {
		t.Error("Decision profile system prompt should mention decision-making components")
	}

	literatureProfile := GetProfile("literature")
	if !containsAll(literatureProfile.SystemPrompt, []string{"literature", "review", "synthesis"}) {
		t.Error("Literature profile system prompt should mention literature review components")
	}
}

// TestReportTypeNormalizationConsistency verifies that the brief package
// and template package normalize report types consistently
func TestReportTypeNormalizationConsistency(t *testing.T) {
	testCases := []struct {
		input    string
		expected Type
	}{
		{"IMRaD", IMRaD},
		{"imrad", IMRaD},
		{"I.M.R.A.D", IMRaD},
		{"Introduction, Methods, Results, Discussion", IMRaD},
		{"decision", Decision},
		{"technical", Decision},
		{"decision report", Decision},
		{"literature", Literature},
		{"literature review", Literature},
		{"systematic review", Literature},
		{"", Default},
		{"unknown", Default},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			profile := GetProfile(tc.input)
			if profile.Type != tc.expected {
				t.Errorf("GetProfile(%q).Type = %v, want %v", tc.input, profile.Type, tc.expected)
			}

			// Also test brief parsing for consistency
			briefText := "# Test Topic\nType: " + tc.input
			parsedBrief := brief.ParseBrief(briefText)
			briefProfile := GetProfile(parsedBrief.ReportType)
			if briefProfile.Type != tc.expected {
				t.Errorf("Brief parsing inconsistency for %q: got %v, want %v", 
					tc.input, briefProfile.Type, tc.expected)
			}
		})
	}
}

// containsAll checks if text contains all required substrings (case-insensitive)
func containsAll(text string, required []string) bool {
	for _, req := range required {
		if !containsIgnoreCase(text, req) {
			return false
		}
	}
	return true
}

// containsIgnoreCase performs case-insensitive substring check
func containsIgnoreCase(text, substr string) bool {
	return len(text) >= len(substr) && 
		   findIgnoreCase(text, substr) >= 0
}

// findIgnoreCase finds substring ignoring case, returns -1 if not found
func findIgnoreCase(text, substr string) int {
	textLower := make([]rune, 0, len(text))
	for _, r := range text {
		if r >= 'A' && r <= 'Z' {
			textLower = append(textLower, r+32)
		} else {
			textLower = append(textLower, r)
		}
	}
	
	substrLower := make([]rune, 0, len(substr))
	for _, r := range substr {
		if r >= 'A' && r <= 'Z' {
			substrLower = append(substrLower, r+32)
		} else {
			substrLower = append(substrLower, r)
		}
	}
	
	if len(substrLower) == 0 {
		return 0
	}
	if len(textLower) < len(substrLower) {
		return -1
	}
	
	for i := 0; i <= len(textLower)-len(substrLower); i++ {
		match := true
		for j := 0; j < len(substrLower); j++ {
			if textLower[i+j] != substrLower[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}