# goresearch CLI reference

This page is auto-generated from the CLI flag definitions.

## Usage

```
goresearch [flags]
goresearch init
goresearch doc
```

## Flags

- `-cache.clear` (default: `false`) — Clear cache directory before run
- `-cache.dir` (default: `.goresearch-cache`) — Cache directory path
- `-cache.maxAge` (default: `0s`) — Max age for cache entries before purge (e.g. 24h, 7d); 0 disables
- `-cache.strictPerms` (default: `false`) — Restrict cache permissions (0700 dirs, 0600 files)
- `-cache.topicHash` (default: ``) — Optional topic hash to scope cache; accepted for traceability
- `-debug-verbose` (default: `false`) — Allow logging raw chain-of-thought (CoT) for debugging Harmony/tool-call interplay
- `-domains.allow` (default: ``) — Comma-separated allowlist of hosts/domains; if set, only these are permitted (subdomains included)
- `-domains.deny` (default: ``) — Comma-separated denylist of hosts/domains; takes precedence over allow
- `-dry-run` (default: `false`) — Plan and select without calling the model
- `-enable.pdf` (default: `false`) — Enable optional PDF ingestion (application/pdf)
- `-input` (default: `request.md`) — Path to input Markdown research request
- `-lang` (default: ``) — Optional language hint, e.g. 'en' or 'fi'
- `-llm.base` (default: ``) — OpenAI-compatible base URL
- `-llm.key` (default: ``) — API key for OpenAI-compatible server
- `-llm.model` (default: ``) — Model name
- `-max.perDomain` (default: `3`) — Maximum sources per domain
- `-max.perSourceChars` (default: `12000`) — Maximum characters per source extract
- `-max.sources` (default: `12`) — Maximum number of sources
- `-min.snippetChars` (default: `0`) — Minimum non-whitespace snippet characters to keep a result (0 disables)
- `-output` (default: `report.md`) — Path to write the final Markdown report
- `-robots.overrideConfirm` (default: `false`) — Second confirmation flag required to activate robots override allowlist
- `-robots.overrideDomains` (default: ``) — Comma-separated domain allowlist to ignore robots.txt (use with --robots.overrideConfirm)
- `-search.file` (default: ``) — Path to JSON file for offline file-based search provider
- `-searx.key` (default: ``) — SearxNG API key (optional)
- `-searx.ua` (default: `goresearch/1.0 (+https://github.com/hyperifyio/goresearch)`) — Custom User-Agent for SearxNG requests
- `-searx.url` (default: ``) — SearxNG base URL
- `-synth.systemPrompt` (default: ``) — Override synthesis system prompt (inline string)
- `-synth.systemPromptFile` (default: ``) — Path to file containing synthesis system prompt
- `-tools.dryRun` (default: `false`) — Do not execute tools; emit dry-run envelopes
- `-tools.enable` (default: `false`) — Enable tool-orchestrated chat mode
- `-tools.maxCalls` (default: `32`) — Max tool calls per run
- `-tools.maxWallClock` (default: `0s`) — Max wall-clock duration for tool loop (e.g. 30s); 0 disables
- `-tools.mode` (default: `harmony`) — Chat protocol mode: harmony|legacy
- `-tools.perToolTimeout` (default: `10s`) — Per-tool execution timeout (e.g. 10s)
- `-v` (default: `false`) — Verbose logging
- `-log.level` (default: ``) — Structured log level for file output: trace|debug|info|warn|error|fatal|panic (default info)
- `-log.file` (default: ``) — Path to write structured JSON logs (default goresearch.log)
- `-verify.systemPrompt` (default: ``) — Override verification system prompt (inline string)
- `-verify.systemPromptFile` (default: ``) — Path to file containing verification system prompt
 - `-verify`/`-no-verify` (default: `-verify`) — Enable or disable the fact-check verification pass and Evidence check appendix

## Environment variables

- `LLM_BASE_URL`: OpenAI-compatible base URL
- `LLM_MODEL`: Model name
- `LLM_API_KEY`: API key
- `SEARX_URL`: SearxNG base URL (or SEARXNG_URL)
- `SEARX_KEY`: SearxNG API key (or SEARXNG_KEY)
- `CACHE_DIR`: Cache directory path
- `LANGUAGE`: Language hint
- `SOURCE_CAPS`: Max sources and optional per-domain cap as '<max>' or '<max>,<perDomain>'
- `CACHE_MAX_AGE`: Purge cache entries older than this duration (e.g. 24h, 7d)
- `DRY_RUN`: Enable dry-run when truthy
- `VERBOSE`: Enable verbose logs when truthy
- `CACHE_CLEAR`: Clear cache before run when truthy
- `CACHE_STRICT_PERMS`: Restrict cache permissions when truthy
- `HTTP_CACHE_ONLY`: Serve HTTP bodies only from cache; fail on miss
- `LLM_CACHE_ONLY`: Serve LLM results only from cache; fail on miss
- `ROBOTS_OVERRIDE_DOMAINS`: Comma-separated allowlist to ignore robots.txt; requires robots.overrideConfirm
- `DOMAINS_ALLOW`: Comma-separated allowlist of hosts/domains
- `DOMAINS_DENY`: Comma-separated denylist of hosts/domains
- `SYNTH_SYSTEM_PROMPT`: Inline synthesis system prompt override
- `SYNTH_SYSTEM_PROMPT_FILE`: Path to synthesis system prompt file
- `VERIFY_SYSTEM_PROMPT`: Inline verification system prompt override
- `VERIFY_SYSTEM_PROMPT_FILE`: Path to verification system prompt file
 - `VERIFY`: Set to truthy to force enable verification (overrides NO_VERIFY)
 - `NO_VERIFY`: Set to truthy to disable verification
- `LOG_LEVEL`: Structured log level for file output (trace|debug|info|warn|error|fatal|panic)
- `LOG_FILE`: Path to write structured JSON logs (default goresearch.log)
- `TOPIC_HASH`: Optional topic hash to scope cache

Generated by `goresearch doc`.
