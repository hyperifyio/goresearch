package extract

// Extractor defines a minimal interface for content extraction strategies.
// Implementations can swap readability tactics without changing callers.
type Extractor interface {
    // Extract converts raw HTML bytes into a simplified Document.
    // Implementations should be deterministic and avoid side effects.
    Extract(input []byte) Document
}

// HeuristicExtractor uses the existing FromHTML function that prefers
// <main>/<article> and applies light boilerplate reduction and normalization.
type HeuristicExtractor struct{}

func (HeuristicExtractor) Extract(input []byte) Document {
    return FromHTML(input)
}
