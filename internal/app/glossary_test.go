package app

import (
    "strings"
    "testing"
)

func TestAppendGlossaryAppendix_AcronymsAndTerms(t *testing.T) {
    md := `# Sample Report
2025-01-01

## Executive summary
We evaluate a Convolutional Neural Network (CNN) for image tasks. The CNN architecture is compared to Vision Transformer (ViT) models.

## Body
A Convolutional Neural Network (CNN) is widely used. CNN training differs from Vision Transformer (ViT).

## References
1. [Paper] https://example.com (2024)
`
    out := appendGlossaryAppendix(md)
    if out == md {
        t.Fatalf("expected glossary appendix to be appended")
    }
    if !containsHeading(out, "glossary") {
        t.Fatalf("glossary heading missing")
    }
    // Acronyms
    if !contains(out, "- CNN — Convolutional Neural Network") {
        t.Fatalf("expected CNN expansion in glossary; got: %s", out)
    }
    if !contains(out, "- ViT — Vision Transformer") {
        t.Fatalf("expected ViT expansion in glossary; got: %s", out)
    }
    // Key terms should include multi-word title-cased phrases that repeat
    if !contains(out, "- Convolutional Neural Network") {
        t.Fatalf("expected key term 'Convolutional Neural Network'")
    }
}

func TestAppendGlossaryAppendix_NoFindings_NoChange(t *testing.T) {
    md := `# Report
2025-01-01

Body text with no acronyms or repeated title-cased terms.

## References
1. [Ref] https://example.com (2023)
`
    out := appendGlossaryAppendix(md)
    if out != md {
        t.Fatalf("expected no changes when no glossary items found")
    }
}

// contains is a small helper for tests
func contains(s, sub string) bool { return strings.Contains(s, sub) }
