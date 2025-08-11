package template

import (
	"strings"
	"testing"
)

func TestGetProfile(t *testing.T) {
	tests := []struct {
		name         string
		reportType   string
		expectedType Type
		expectedName string
	}{
		{"IMRaD exact", "imrad", IMRaD, "IMRaD Report"},
		{"IMRaD full form", "Introduction, Methods, Results, Discussion", IMRaD, "IMRaD Report"},
		{"IMRaD dotted", "I.M.R.A.D", IMRaD, "IMRaD Report"},
		{"IMRaD spaced", "i m r a d", IMRaD, "IMRaD Report"},
		{"IMRaD partial match", "some imrad study", IMRaD, "IMRaD Report"},
		
		{"Decision exact", "decision", Decision, "Technical Decision Report"},
		{"Decision report", "decision report", Decision, "Technical Decision Report"},
		{"Technical", "technical", Decision, "Technical Decision Report"},
		{"Tech report", "tech", Decision, "Technical Decision Report"},
		{"Technical decision", "technical decision", Decision, "Technical Decision Report"},
		{"Decision/tech", "decision/tech", Decision, "Technical Decision Report"},
		{"Decision partial", "make a decision about", Decision, "Technical Decision Report"},
		
		{"Literature exact", "literature", Literature, "Literature Review"},
		{"Literature review", "literature review", Literature, "Literature Review"},
		{"Lit review", "lit review", Literature, "Literature Review"},
		{"Systematic review", "systematic review", Literature, "Literature Review"},
		{"Review", "review", Literature, "Literature Review"},
		{"Literature partial", "literature survey", Literature, "Literature Review"},
		
		{"Empty string", "", Default, "General Report"},
		{"Unknown type", "unknown", Default, "General Report"},
		{"Whitespace", "  \t\n  ", Default, "General Report"},
		{"Mixed case unknown", "SomeOtherType", Default, "General Report"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := GetProfile(tt.reportType)
			if profile.Type != tt.expectedType {
				t.Errorf("GetProfile(%q) type = %v, want %v", tt.reportType, profile.Type, tt.expectedType)
			}
			if profile.Name != tt.expectedName {
				t.Errorf("GetProfile(%q) name = %q, want %q", tt.reportType, profile.Name, tt.expectedName)
			}
		})
	}
}

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"IMRaD", "imrad"},
		{"IMRAD", "imrad"},
		{"imrad", "imrad"},
		{"I.M.R.A.D", "imrad"},
		{"i m r a d", "imrad"},
		{"Introduction, Methods, Results, Discussion", "imrad"},
		
		{"decision", "decision"},
		{"DECISION", "decision"},
		{"Decision Report", "decision"},
		{"technical", "decision"},
		{"tech", "decision"},
		{"Technical Decision", "decision"},
		{"decision/tech", "decision"},
		
		{"literature", "literature"},
		{"LITERATURE", "literature"},
		{"Literature Review", "literature"},
		{"lit review", "literature"},
		{"systematic review", "literature"},
		{"review", "literature"},
		
		{"", ""},
		{"unknown", ""},
		{"random text", ""},
		{"  \t\n  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeType(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIMRaDProfile(t *testing.T) {
	profile := imradProfile()
	
	if profile.Type != IMRaD {
		t.Errorf("IMRaD profile type = %v, want %v", profile.Type, IMRaD)
	}
	
	expectedSections := []string{
		"Executive summary",
		"Introduction", 
		"Methods",
		"Results",
		"Discussion",
		"Alternatives & conflicting evidence",
		"Risks and limitations",
		"References",
	}
	
	if len(profile.Outline) != len(expectedSections) {
		t.Fatalf("IMRaD profile outline length = %d, want %d", len(profile.Outline), len(expectedSections))
	}
	
	for i, expected := range expectedSections {
		if profile.Outline[i] != expected {
			t.Errorf("IMRaD profile outline[%d] = %q, want %q", i, profile.Outline[i], expected)
		}
	}
	
	// Verify system prompt mentions IMRaD structure
	if !strings.Contains(profile.SystemPrompt, "IMRaD") {
		t.Error("IMRaD profile system prompt should mention IMRaD structure")
	}
	if !strings.Contains(profile.SystemPrompt, "Introduction") {
		t.Error("IMRaD profile system prompt should mention Introduction")
	}
	if !strings.Contains(profile.SystemPrompt, "Methods") {
		t.Error("IMRaD profile system prompt should mention Methods")
	}
	if !strings.Contains(profile.SystemPrompt, "Results") {
		t.Error("IMRaD profile system prompt should mention Results")
	}
	if !strings.Contains(profile.SystemPrompt, "Discussion") {
		t.Error("IMRaD profile system prompt should mention Discussion")
	}
}

func TestDecisionProfile(t *testing.T) {
	profile := decisionProfile()
	
	if profile.Type != Decision {
		t.Errorf("Decision profile type = %v, want %v", profile.Type, Decision)
	}
	
	expectedSections := []string{
		"Executive summary",
		"Problem statement",
		"Decision criteria", 
		"Options evaluated",
		"Recommendation",
		"Implementation considerations",
		"Alternatives & conflicting evidence",
		"Risks and limitations",
		"References",
	}
	
	if len(profile.Outline) != len(expectedSections) {
		t.Fatalf("Decision profile outline length = %d, want %d", len(profile.Outline), len(expectedSections))
	}
	
	for i, expected := range expectedSections {
		if profile.Outline[i] != expected {
			t.Errorf("Decision profile outline[%d] = %q, want %q", i, profile.Outline[i], expected)
		}
	}
	
	// Verify system prompt mentions decision structure
	if !strings.Contains(profile.SystemPrompt, "decision") {
		t.Error("Decision profile system prompt should mention decision structure")
	}
	if !strings.Contains(profile.SystemPrompt, "problem") {
		t.Error("Decision profile system prompt should mention problem")
	}
	if !strings.Contains(profile.SystemPrompt, "recommendation") {
		t.Error("Decision profile system prompt should mention recommendation")
	}
}

func TestLiteratureProfile(t *testing.T) {
	profile := literatureProfile()
	
	if profile.Type != Literature {
		t.Errorf("Literature profile type = %v, want %v", profile.Type, Literature)
	}
	
	expectedSections := []string{
		"Executive summary",
		"Background and scope",
		"Review methodology",
		"Thematic analysis",
		"Key findings synthesis", 
		"Research gaps identified",
		"Alternatives & conflicting evidence",
		"Risks and limitations",
		"References",
	}
	
	if len(profile.Outline) != len(expectedSections) {
		t.Fatalf("Literature profile outline length = %d, want %d", len(profile.Outline), len(expectedSections))
	}
	
	for i, expected := range expectedSections {
		if profile.Outline[i] != expected {
			t.Errorf("Literature profile outline[%d] = %q, want %q", i, profile.Outline[i], expected)
		}
	}
	
	// Verify system prompt mentions literature review structure
	if !strings.Contains(profile.SystemPrompt, "literature") {
		t.Error("Literature profile system prompt should mention literature")
	}
	if !strings.Contains(profile.SystemPrompt, "review") {
		t.Error("Literature profile system prompt should mention review")
	}
	if !strings.Contains(profile.SystemPrompt, "synthesis") {
		t.Error("Literature profile system prompt should mention synthesis")
	}
}

func TestDefaultProfile(t *testing.T) {
	profile := defaultProfile()
	
	if profile.Type != Default {
		t.Errorf("Default profile type = %v, want %v", profile.Type, Default)
	}
	
	expectedSections := []string{
		"Executive summary",
		"Background",
		"Core concepts", 
		"Implementation guidance",
		"Examples",
		"Alternatives & conflicting evidence",
		"Risks and limitations",
		"References",
	}
	
	if len(profile.Outline) != len(expectedSections) {
		t.Fatalf("Default profile outline length = %d, want %d", len(profile.Outline), len(expectedSections))
	}
	
	for i, expected := range expectedSections {
		if profile.Outline[i] != expected {
			t.Errorf("Default profile outline[%d] = %q, want %q", i, profile.Outline[i], expected)
		}
	}
}

func TestAllProfilesHaveRequiredSections(t *testing.T) {
	profiles := []Profile{
		imradProfile(),
		decisionProfile(), 
		literatureProfile(),
		defaultProfile(),
	}
	
	requiredSections := []string{
		"Executive summary",
		"Alternatives & conflicting evidence",
		"Risks and limitations",
		"References",
	}
	
	for _, profile := range profiles {
		t.Run(profile.Name, func(t *testing.T) {
			for _, required := range requiredSections {
				found := false
				for _, section := range profile.Outline {
					if strings.EqualFold(section, required) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Profile %s missing required section: %s", profile.Name, required)
				}
			}
		})
	}
}

func TestProfileSystemPromptsHaveBasics(t *testing.T) {
	profiles := []Profile{
		imradProfile(),
		decisionProfile(),
		literatureProfile(), 
		defaultProfile(),
	}
	
	requiredPhrases := []string{
		"Use ONLY the provided sources",
		"Cite precisely", 
		"bracketed numeric indices",
		"Do not invent sources",
	}
	
	for _, profile := range profiles {
		t.Run(profile.Name, func(t *testing.T) {
			for _, phrase := range requiredPhrases {
				if !strings.Contains(profile.SystemPrompt, phrase) {
					t.Errorf("Profile %s system prompt missing phrase: %s", profile.Name, phrase)
				}
			}
		})
	}
}

// Test property: all profiles should have distinct section structures
func TestProfilesHaveDistinctStructures(t *testing.T) {
	profiles := map[string]Profile{
		"imrad":      imradProfile(),
		"decision":   decisionProfile(),
		"literature": literatureProfile(),
		"default":    defaultProfile(),
	}
	
	// Each profile should have unique core sections (excluding common ones)
	commonSections := map[string]bool{
		"Executive summary":                     true,
		"Alternatives & conflicting evidence":  true,
		"Risks and limitations":                true,
		"References":                           true,
	}
	
	for name1, profile1 := range profiles {
		for name2, profile2 := range profiles {
			if name1 >= name2 { // avoid duplicate comparisons
				continue
			}
			
			// Extract unique sections for each profile
			unique1 := []string{}
			unique2 := []string{}
			
			for _, section := range profile1.Outline {
				if !commonSections[section] {
					unique1 = append(unique1, section)
				}
			}
			
			for _, section := range profile2.Outline {
				if !commonSections[section] {
					unique2 = append(unique2, section)
				}
			}
			
			// Profiles should have different unique sections
			if len(unique1) == 0 || len(unique2) == 0 {
				t.Errorf("Profiles %s and %s should have unique sections beyond common ones", name1, name2)
				continue
			}
			
			// Check if any unique sections overlap
			overlap := false
			for _, s1 := range unique1 {
				for _, s2 := range unique2 {
					if strings.EqualFold(s1, s2) {
						overlap = true
						break
					}
				}
				if overlap {
					break
				}
			}
			
			if overlap {
				t.Logf("Note: Profiles %s and %s have overlapping unique sections - this may be acceptable", name1, name2)
			}
		}
	}
}