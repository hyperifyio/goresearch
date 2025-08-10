# Verification evidence appendix and manifest guide

This guide explains how goresearch produces the Evidence check appendix, how to read it, and how to use the embedded and sidecar manifest for audits and reproducibility.

## What the Evidence check does
- **Purpose**: Summarize key factual claims in the generated report, map each to the numbered sources from the References section, and assign a confidence label.
- **Source**: A short verification pass analyzes the final Markdown and extracts 5–12 claims, associating supporting citation indices.
- **Graceful failure**: If verification is unavailable or fails, the main report is preserved and no appendix is added.

## Where it appears in the report
The appendix is appended near the end of `report.md` under the heading `## Evidence check`.

It contains:
- An optional one-paragraph summary like `8 claims extracted; 6 supported by citations; 2 low-confidence.`
- Up to 20 bullet lines, one per claim:
  - Format: `- <claim text> — cites [n[,m,…]]; confidence: high|medium|low; supported: true|false`
  - Citation numbers correspond to the numbered entries in `## References`.

Example snippet:

```markdown
## Evidence check

8 claims extracted; 6 supported by citations; 2 low-confidence.

- Protocol X was standardized in 2018 [1] — cites [1]; confidence: medium; supported: true
- Claimed 30% performance gains [2][3] — cites [2,3]; confidence: high; supported: true
- Y is universally faster in all cases — cites []; confidence: low; supported: false
```

## How confidence is assigned
- **High**: multiple citations ([n][m]) support the claim
- **Medium**: a single citation ([n]) supports the claim
- **Low**: no citations map to the claim (often marked `supported: false`)

Note: Exact heuristics may vary when an LLM verifier is enabled. A deterministic fallback guarantees stable behavior when the LLM is not used.

## The embedded manifest (human-readable)
Every report includes a `## Manifest` section that records the run and the exact sources used:
- **Header fields**: model, LLM base URL, number of sources, cache flags, generated timestamp (UTC)
- **Entries**: one line per source, using its canonical URL and a SHA-256 digest of the precise excerpt used during synthesis, plus its character count

Example lines:

```markdown
## Manifest

- Model: test-model
- LLM base URL: http://localhost:11434/v1
- Sources: 2
- HTTP cache: true
- LLM cache: true
- Generated: 2000-01-01T00:00:00Z

1. https://example.com/spec — sha256=...; chars=1234
2. https://example.org/ref  — sha256=...; chars=987
```

This lets readers verify that a specific version of each source was used, even if the page later changes.

## The sidecar manifest (machine-readable JSON)
Alongside `report.md`, a sidecar JSON file is written with the same path plus `.manifest.json`, e.g. `report.md.manifest.json`. It includes:
- `meta`: the same header fields as the embedded manifest
- `sources`: an array with `index`, `url`, `title`, `sha256`, and `chars`

Example (trimmed):

```json
{
  "meta": {
    "model": "test-model",
    "llm_base_url": "http://localhost:11434/v1",
    "source_count": 2,
    "http_cache": true,
    "llm_cache": true,
    "generated_at": "2000-01-01T00:00:00Z"
  },
  "sources": [
    {"index": 1, "url": "https://example.com/spec", "title": "Spec", "sha256": "...", "chars": 1234},
    {"index": 2, "url": "https://example.org/ref",  "title": "Ref",  "sha256": "...", "chars": 987}
  ]
}
```

Tip: Use the sidecar JSON to script reproducibility checks or auditing pipelines.

## Skipped URLs due to robots/opt-out
When URLs are skipped because of robots or AI/TDM opt-out signals, the manifest includes a `### Skipped due to robots/opt-out` section listing each skipped URL and the reason. This provides a clear audit trail of compliance decisions.

## Tool-call transcript (when tool mode is enabled)
When the tool-orchestrated mode is used, the report may include a `### Tool-call transcript` section. Each line records a tool invocation with:
- ordinal number and tool name
- tool call ID and outcome flags (`ok`, `dry_run`)
- stable hashes of arguments and results, plus byte counts

Example line:

```text
1. web_search (id=tc1, ok=true, dry_run=false) args_hash=aabb result_sha256=ccdd result_bytes=123
```

Use this transcript to correlate planning and fetching actions with the final manifest and to reproduce the sequence of external interactions.

## How to audit a report
1. Open `report.md` and locate `## References`, `## Evidence check`, and `## Manifest`.
2. For any claim in the Evidence check, compare its cited indices to the corresponding entries in `## References`.
3. Cross-check a source line in the Manifest by recomputing the SHA-256 of the excerpt used. Because the manifest digests are computed on the exact excerpt fed to the model, you can:
   - Retrieve the page at the reference URL.
   - Extract the relevant text region approximating the report’s cited content.
   - Compute `sha256` over the normalized excerpt to match the manifest entry.
4. For automated workflows, parse `report.md.manifest.json` and validate each `sha256` and URL.

## Troubleshooting
- If `## Evidence check` is missing, verification likely failed or was disabled; the main analysis remains intact.
- If `## Manifest` is missing, ensure you are reading the final `report.md` produced by goresearch (not an intermediate draft).
- If digests don’t match, confirm you normalized text consistently (trim whitespace) and you’re comparing against the excerpt actually used during synthesis.
