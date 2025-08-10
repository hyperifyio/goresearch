package brief

import (
	"bufio"
	"regexp"
	"strings"
)

// Brief represents the distilled research request parsed from a single
// Markdown input. It intentionally keeps only the fields the rest of the
// pipeline needs.
type Brief struct {
	Topic        string
	AudienceHint string
	ToneHint     string
	// TargetLengthWords is a soft target. Zero means unspecified.
	TargetLengthWords int
    // ReportType selects a section profile (e.g., "imrad", "decision", "literature").
    ReportType string
	// Raw is the original input for traceability if needed downstream.
	Raw string
}

var (
	headingRe      = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	audienceLineRe = regexp.MustCompile(`(?i)^\s*audience\s*[:\-]\s*(.+?)\s*$`)
	toneLineRe     = regexp.MustCompile(`(?i)^\s*tone\s*[:\-]\s*(.+?)\s*$`)
	// Examples: "target length: 1200 words", "~800 words", "max 1500 words"
	wordsRe = regexp.MustCompile(`(?i)(?:target\s*length|~|about|approx\.?|max)?\s*([0-9]{2,5})\s*(?:word|words)\b`)
    // Report type indicators: "Type: IMRaD", "Report type: literature review", "Profile: decision report"
    reportTypeLineRe = regexp.MustCompile(`(?i)^\s*(?:type|report\s*type|profile)\s*[:\-]\s*(.+?)\s*$`)
)

// ParseBrief parses a Markdown string into a Brief. The parser is deliberately
// conservative and deterministic: it looks for the first heading as the topic,
// otherwise falls back to the first non-empty line stripped of markdown noise.
// It scans for audience/tone hints and an optional target word-count.
func ParseBrief(input string) Brief {
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(bufio.ScanLines)

	brief := Brief{Raw: input}
	var firstNonEmpty string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if brief.Topic == "" {
			if m := headingRe.FindStringSubmatch(trimmed); len(m) == 2 {
				brief.Topic = strings.TrimSpace(stripTrailingPunctuation(m[1]))
			}
		}
		if firstNonEmpty == "" {
			firstNonEmpty = trimmed
		}

		if brief.AudienceHint == "" {
			if m := audienceLineRe.FindStringSubmatch(trimmed); len(m) == 2 {
				brief.AudienceHint = strings.TrimSpace(m[1])
			}
		}
		if brief.ToneHint == "" {
			if m := toneLineRe.FindStringSubmatch(trimmed); len(m) == 2 {
				brief.ToneHint = strings.TrimSpace(m[1])
			}
		}

        if brief.ReportType == "" {
            if m := reportTypeLineRe.FindStringSubmatch(trimmed); len(m) == 2 {
                brief.ReportType = normalizeReportType(strings.TrimSpace(m[1]))
            }
        }

		if brief.TargetLengthWords == 0 {
			if m := wordsRe.FindStringSubmatch(trimmed); len(m) == 2 {
				brief.TargetLengthWords = parseIntSafe(m[1])
			}
		}
	}

	if brief.Topic == "" {
		// Fallback: use the first non-empty line stripped of markdown markers.
		brief.Topic = deriveTopicFromLine(firstNonEmpty)
	}

	// Sensible default if still zero; callers may treat zero as unspecified.
	if brief.TargetLengthWords == 0 {
		brief.TargetLengthWords = 1200
	}

	return brief
}

func normalizeReportType(s string) string {
    v := strings.ToLower(strings.TrimSpace(s))
    switch v {
    case "imrad", "i.m.r.a.d", "i m r a d", "introduction, methods, results, discussion":
        return "imrad"
    case "decision", "decision report", "tech", "technical", "technical report", "technical decision", "decision/tech", "decision tech":
        return "decision"
    case "literature", "literature review", "lit review", "systematic review", "review":
        return "literature"
    default:
        // Try to map substrings conservatively
        if strings.Contains(v, "imrad") {
            return "imrad"
        }
        if strings.Contains(v, "decision") || strings.Contains(v, "technical") || strings.Contains(v, "tech") {
            return "decision"
        }
        if strings.Contains(v, "review") || strings.Contains(v, "literature") {
            return "literature"
        }
        return ""
    }
}

func deriveTopicFromLine(line string) string {
	if line == "" {
		return ""
	}
	// Remove simple markdown markers like emphasis or inline code wrappers.
	s := strings.TrimSpace(line)
	s = strings.Trim(s, "`*")
	s = stripTrailingPunctuation(s)
	return s
}

func stripTrailingPunctuation(s string) string {
	return strings.TrimRight(s, " #:-")
}

func parseIntSafe(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return n
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
