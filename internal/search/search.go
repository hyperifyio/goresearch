package search

import (
	"context"
)

// Result represents a single search hit from any provider.
type Result struct {
	Title   string
	URL     string
	Snippet string
	Source  string // provider name for observability
}

// Provider is a minimal interface for search providers.
type Provider interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
	Name() string
}

// DomainPolicy allows providers to filter or block results/requests by host.
// Implementations should treat Denylist as taking precedence over Allowlist.
type DomainPolicy struct {
    Allowlist []string
    Denylist  []string
}
