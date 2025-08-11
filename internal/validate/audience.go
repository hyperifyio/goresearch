package validate

import (
    "fmt"
    "regexp"
    "strings"
)

// ValidateAudienceFit checks that the generated markdown matches the intended
// audience and tone hints. It flags:
// - For non-technical audiences: high jargon/acronym density, code blocks, or
//   deeply technical sections (e.g., Implementation, API) that are likely
//   mismatched.
// - For formal tone: casual language markers.
// The function is deterministic and conservative to avoid false positives.
func ValidateAudienceFit(markdown, audienceHint, toneHint string) error {
    aud := classifyAudience(audienceHint)
    tone := classifyTone(toneHint)

    lines := splitLines(markdown)

    // Build sections from headings
    type section struct{ title string; start, end int }
    var sections []section
    // Find all headings and ranges
    for i := 0; i < len(lines); i++ {
        if isHeading(trimSpace(lines[i])) {
            // collect previous section end
            if len(sections) > 0 {
                sections[len(sections)-1].end = i
            }
            sections = append(sections, section{title: stripHeading(trimSpace(lines[i])), start: i + 1, end: len(lines)})
        }
    }
    if len(sections) == 0 {
        // Consider whole doc as one section
        sections = append(sections, section{title: "", start: 0, end: len(lines)})
    } else {
        // Ensure last section end is set
        if sections[len(sections)-1].end == 0 {
            sections[len(sections)-1].end = len(lines)
        }
    }

    var issues []string

    // Tone check: casual markers under formal tone
    if tone == toneFormal {
        if hasCasualMarkers(markdown) {
            issues = append(issues, "casual tone markers present despite 'formal' tone hint")
        }
    }

    // Audience checks
    if aud == audienceNonTechnical {
        // Global red flags for non-technical:
        if containsCodeBlock(lines) {
            issues = append(issues, "code blocks present for non-technical audience")
        }
    }

    // Section-level jargon density and mismatched section titles
    for _, sec := range sections {
        title := strings.ToLower(trimSpace(sec.title))
        body := strings.Join(lines[sec.start:sec.end], "\n")

        if aud == audienceNonTechnical {
            if isTechnicalSectionTitle(title) {
                issues = append(issues, fmt.Sprintf("section '%s' appears too technical for non-technical audience", safeTitle(sec.title)))
            }
            jd := jargonDensity(body)
            // Heuristic threshold: >4 jargon/acronym hits per 100 words is high
            if jd > 4.0 {
                issues = append(issues, fmt.Sprintf("section '%s' has high jargon density (%.1f per 100 words)", safeTitle(sec.title), jd))
            }
        }
    }

    if len(issues) == 0 {
        return nil
    }
    return fmt.Errorf("audience fit issues: %s", strings.Join(issues, "; "))
}

type audienceClass int
const (
    audienceUnknown audienceClass = iota
    audienceNonTechnical
    audienceTechnical
)

type toneClass int
const (
    toneUnknown toneClass = iota
    toneFormal
    toneConversational
)

func classifyAudience(hint string) audienceClass {
    s := strings.ToLower(strings.TrimSpace(hint))
    if s == "" { return audienceUnknown }
    nonTechMarkers := []string{"executive", "stakeholder", "non-technical", "nontechnical", "business", "manager", "leadership", "cxo", "c-level", "general audience", "layperson", "students", "beginners"}
    for _, k := range nonTechMarkers {
        if strings.Contains(s, k) {
            return audienceNonTechnical
        }
    }
    techMarkers := []string{"engineer", "developer", "architect", "researcher", "scientist", "devops", "sre", "security", "data", "ml", "phd", "expert", "advanced", "technical"}
    for _, k := range techMarkers {
        if strings.Contains(s, k) {
            return audienceTechnical
        }
    }
    return audienceUnknown
}

func classifyTone(hint string) toneClass {
    s := strings.ToLower(strings.TrimSpace(hint))
    if s == "" { return toneUnknown }
    if strings.Contains(s, "formal") || strings.Contains(s, "professional") {
        return toneFormal
    }
    if strings.Contains(s, "conversational") || strings.Contains(s, "casual") || strings.Contains(s, "friendly") {
        return toneConversational
    }
    return toneUnknown
}

func containsCodeBlock(lines []string) bool {
    fence := "```"
    for _, l := range lines {
        if strings.Contains(l, fence) {
            return true
        }
    }
    return false
}

var acronymRe = regexp.MustCompile(`\b[A-Z]{2,6}\b`)

// Minimal list of technical jargon tokens. Lowercase compare.
var jargonLexicon = []string{
    "throughput","latency","idempotent","eventual consistency","strong consistency","cap theorem","sharding","partitioning","vectorization","backpropagation","bayesian","kubernetes","containerization","orchestration","rpc","sdk","cli","api","etl","schema","serialization","protobuf","grpc","rest","oauth","jwt","oidc","monoid","functor","microservices","observability","telemetry","tracing","rate limiting","circuit breaker","consensus","raft","paxos","quorum","saga","acos","big-o","asymptotic","lock-free","wait-free","contention","memory model","coherence","cache invalidation",
}

func isTechnicalSectionTitle(title string) bool {
    if title == "" { return false }
    t := strings.ToLower(title)
    candidates := []string{"implementation", "implementation details", "api", "code", "configuration", "algorithm", "protocol details", "data model", "schema", "deployment", "benchmark", "performance tuning"}
    for _, c := range candidates { if strings.Contains(t, c) { return true } }
    return false
}

func jargonDensity(text string) float64 {
    words := CountWords(text)
    if words == 0 { return 0 }
    hits := 0
    // Acronyms
    for _, m := range acronymRe.FindAllString(text, -1) {
        // whitelist a few common short forms that are broadly known
        switch m {
        case "AI", "CPU", "RAM", "HTTP", "URL", "SQL":
            continue
        }
        hits++
    }
    // Lexicon items
    low := strings.ToLower(text)
    for _, j := range jargonLexicon {
        if strings.Contains(low, j) {
            hits++
        }
    }
    per100 := (float64(hits) / float64(words)) * 100.0
    return per100
}

func CountWords(s string) int {
    // Simple split on whitespace; collapse multiples
    n := 0
    in := false
    for i := 0; i < len(s); i++ {
        b := s[i]
        if b == ' ' || b == '\n' || b == '\t' || b == '\r' {
            if in { n++; in = false }
        } else {
            in = true
        }
    }
    if in { n++ }
    return n
}

func hasCasualMarkers(s string) bool {
    low := strings.ToLower(s)
    markers := []string{"awesome", "cool", "super", "kinda", "sort of", "you guys", "!", "btw", "asap", "lol"}
    for _, m := range markers {
        if strings.Contains(low, m) { return true }
    }
    return false
}

func safeTitle(s string) string {
    t := strings.TrimSpace(s)
    if t == "" { return "(untitled)" }
    return t
}
