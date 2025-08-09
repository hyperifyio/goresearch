package brief

import "testing"

func TestParseBrief_HeadingTopic_AudienceTone_Words(t *testing.T) {
	input := `# Cursor MDC format

Audience: power users and plugin authors
Tone: concise technical
Target length: 1500 words

Details about what to cover...`

	b := ParseBrief(input)

	if b.Topic != "Cursor MDC format" {
		t.Fatalf("topic: got %q", b.Topic)
	}
	if b.AudienceHint != "power users and plugin authors" {
		t.Fatalf("audience: got %q", b.AudienceHint)
	}
	if b.ToneHint != "concise technical" {
		t.Fatalf("tone: got %q", b.ToneHint)
	}
	if b.TargetLengthWords != 1500 {
		t.Fatalf("TargetLengthWords: got %d", b.TargetLengthWords)
	}
}

func TestParseBrief_Fallbacks_DefaultLength(t *testing.T) {
	input := `Investigate Go 1.23 profile-guided optimizations`
	b := ParseBrief(input)
	if b.Topic != "Investigate Go 1.23 profile-guided optimizations" {
		t.Fatalf("topic: got %q", b.Topic)
	}
	if b.TargetLengthWords == 0 {
		t.Fatalf("expected default length, got 0")
	}
}
