# Troubleshooting & FAQ

This guide covers common failures and how to increase verbosity to debug runs.

## Quick debug: increase logging

- Use verbose console output and debug log level:

```bash
goresearch -v -log.level debug -input request.md -output report.md \
  -llm.base "$LLM_BASE_URL" -llm.model "$LLM_MODEL" -llm.key "$LLM_API_KEY" \
  -searx.url "$SEARX_URL"
```

- Environment alternatives:
  - `VERBOSE=1` to mirror `-v`
  - `LOG_LEVEL=debug` to mirror `-log.level debug`
  - `LOG_FILE=path/to/goresearch.log` to mirror `-log.file`

- For deep protocol troubleshooting, enable prompt logging (may include raw prompts; never enable in production):

```bash
goresearch -debug-verbose ...
```

The CLI prints concise progress to stderr. Structured JSON logs go to `goresearch.log` by default (or `-log.file`). Tail them during a run:

```bash
tail -f goresearch.log | sed -E 's/\\n/ /g'
```

## Cache issues

Symptoms and fixes related to the local cache directory (default `.goresearch-cache`).

- Stale or surprising results:
  - Purge by age: `-cache.maxAge 24h`
  - Clear entirely before a run: `-cache.clear`

- “http cache-only: not found” or “http cache-only: not found meta”
  - You enabled `HTTP_CACHE_ONLY` or `-httpCacheOnly` via env/flags. In this mode, network is disabled and a miss is a hard error. Either pre-seed the cache or disable cache-only mode for that run.

- Permission errors writing cache on some filesystems:
  - Set a custom path: `-cache.dir /path/you/own`
  - Restrict permissions if your environment requires it: `-cache.strictPerms` (0700 dirs / 0600 files)

- Cache growing too large:
  - Enforce limits: `CACHE_MAX_BYTES=2147483648 CACHE_MAX_COUNT=5000 goresearch ...`

## Robots and opt-out denials

The tool proactively skips URLs that violate robots/opt-out policies. You’ll see structured log entries explaining the decision.

- Common reasons for skipping:
  - robots.txt Disallow matched for your User-Agent
  - Temporary disallow due to robots.txt 401/403/5xx or timeout
  - AI/TDM opt-out signals: `X-Robots-Tag: noai|notrain`, HTML meta `noai|notrain`, `Link: rel="tdm-reservation"`, or `<link rel="tdm-reservation">`

- How to inspect decisions:
  - Run with `-log.level debug` and check `goresearch.log` for fields like `robots_agent`, `matched_rule`, and `opt_out_signal`.

- Overriding robots.txt for specific domains (for controlled environments only):

```bash
goresearch -robots.overrideDomains example.com,docs.internal -robots.overrideConfirm ...
```

Both flags are required. This does not override AI/TDM opt-out signals (`noai`, `notrain`, TDM reservation), which remain enforced.

## LLM endpoint issues

Typical errors and remedies when connecting to an OpenAI-compatible server.

- “connection refused”, timeouts, or TLS errors:
  - Verify the base URL is reachable:
    ```bash
    curl -sS "$LLM_BASE_URL/v1/models"
    ```
  - Check local firewall or proxy settings.

- “model not found”:
  - Ensure `-llm.model` matches an installed/served model on your server.
  - Consult your LLM server docs for loading models.

- 401/403 Unauthorized:
  - Set `-llm.key "$LLM_API_KEY"` or `LLM_API_KEY` env correctly.
  - Some servers accept any string but still require a header; pass a non-empty key.

- Rate limits or slow responses:
  - Keep runs deterministic and light; retry by re-running. The synthesizer performs one short backoff retry on transient errors.

## FAQ

- Why did a URL get skipped before fetching?
  - Check robots/opt-out denials above. The decision is logged with the matched directive. Use the robots override allowlist only in controlled environments, and note that opt-out cannot be bypassed.

- How do I run without network for strict reproducibility?
  - Use `HTTP_CACHE_ONLY=1 LLM_CACHE_ONLY=1` and ensure the caches are populated. Misses will fail fast.

- Where do logs go and how do I change the path?
  - Default is `goresearch.log` in the working directory. Change with `-log.file` or `LOG_FILE`.

- Can I see the exact prompts sent to the model?
  - Yes, with `-debug-verbose`. Use only for local debugging; this may include long excerpts and raw prompts.
