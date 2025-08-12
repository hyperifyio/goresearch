# goresearch

Generate validated, citation-rich research reports from a single Markdown brief. goresearch plans queries, searches the web (optionally via SearxNG), fetches and extracts readable text, and can call an external OpenAI-compatible LLM to synthesize a clean Markdown report with numbered citations, references, and an evidence-check appendix. Local LLM containers are not provided.

### Table of contents
- [Features](#features)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Usage](#usage)
- [Caching and reproducibility](#caching-and-reproducibility)
- [Verification & manifest guide](#verification--manifest-guide)
- [Robots, opt-out, and politeness policy](#robots-opt-out-and-politeness-policy)
- [Tests](#tests)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Support](#support)
- [License](#license)
- [Project status](#project-status)
 - [Full CLI reference](#full-cli-reference)
 - [Run locally with Docker](#run-locally-with-docker)
 - [Local stack helpers (optional)](#local-stack-helpers-optional)
 - [Troubleshooting & FAQ](#troubleshooting--faq)

## Features
- **End-to-end pipeline**: brief parsing → planning → search → fetch/extract → selection/dedup → budgeting → synthesis → validation → verification → rendering.
- **Grounded synthesis**: strictly uses supplied extracts; numbered inline citations map to a final References section.
- **Evidence check**: second pass extracts claims, maps supporting sources, and flags weakly supported statements.
- **Deterministic and scriptable**: low-temperature prompts, structured logs, and explicit flags/envs.
- **Pluggable search**: defaults to self-hosted SearxNG; adapters can be swapped without changing the rest.
- **Polite fetching**: user agent, timeouts, redirect caps, content-type checks, and optional HTTP cache with conditional requests.
- **Public web only**: blocks localhost/private IPs and URL-embedded credentials to avoid private services.
- **Token budgeting**: proportional truncation prevents dropping sources while fitting model context.
- **Reproducibility**: embedded manifest and sidecar JSON record URLs and content digests used in synthesis.
- **Dry run**: plan queries and select URLs without calling the model.

## Installation

Prerequisites:
- Go 1.23+ (module toolchain `go1.24.6`)
- An OpenAI-compatible server (local OSS runtime recommended), with model name and API key
- Optional: a SearxNG instance URL (and API key if required)

Install the CLI directly:

```bash
go install github.com/hyperifyio/goresearch/cmd/goresearch@latest
```

Or build from source:

```bash
git clone https://github.com/hyperifyio/goresearch
cd goresearch
go build -o bin/goresearch ./cmd/goresearch
```

## Quick start

### One‑liner (deterministic dry run)

Copy and paste this command. It writes a small “hello research” brief, runs a dry run (no LLM required), and prints the beginning of the resulting Markdown report.

```bash
printf "%s\n" "# Hello Research — Brief introduction to goresearch" "" \
  "Audience: Developers and researchers" \
  "Tone: Practical, welcoming" \
  "Target length: 800 words" "" \
  "Key questions: What is goresearch? How does it work? What makes it useful for researchers and developers?" \
  > hello-research.md && \
goresearch -dry-run -input hello-research.md -output hello-research-report.md && \
sed -n '1,24p' hello-research-report.md
```

Expected output (first lines):

```markdown
# goresearch (dry run)

Topic: Hello Research — Brief introduction to goresearch
Audience: Developers and researchers
Tone: Practical, welcoming
Target Length (words): 800

Planned queries:
1. Hello Research — Brief introduction to goresearch specification
2. Hello Research — Brief introduction to goresearch documentation
3. Hello Research — Brief introduction to goresearch reference
4. Hello Research — Brief introduction to goresearch tutorial
5. Hello Research — Brief introduction to goresearch best practices
6. Hello Research — Brief introduction to goresearch faq
7. Hello Research — Brief introduction to goresearch examples
8. Hello Research — Brief introduction to goresearch comparison
9. Hello Research — Brief introduction to goresearch limitations
10. Hello Research — Brief introduction to goresearch contrary findings

Selected URLs:
1. Hello Research — Brief introduction to goresearch specification — https://github.com/hyperifyio/goresearch
2. Hello Research — Brief introduction to goresearch reference — https://goresearch.dev/reference
3. Hello Research — Brief introduction to goresearch documentation — https://goresearch.dev/documentation
4. Hello Research — Brief introduction to goresearch tutorial — https://goresearch.dev/tutorial
```

Tip: remove `-dry-run` and set `LLM_BASE_URL` (e.g., `https://your-llm.example.com/v1`), `LLM_MODEL` (e.g., `your/model-id`), and `LLM_API_KEY` (if your server requires it). `SEARX_URL` is optional.

### “Hello research” brief and result

Brief used above:

```markdown
# Hello Research — Brief introduction to goresearch

Audience: Developers and researchers
Tone: Practical, welcoming  
Target length: 800 words

Key questions: What is goresearch? How does it work? What makes it useful for researchers and developers?
```

Result (dry run) is written to `hello-research-report.md`. See also the committed sample at `hello-research-report.md` and `reports/hello-research-brief-introduction-to-goresearch/report.md`.
1) Create a minimal `request.md` with topic and optional hints:

```markdown
# Cursor MDC format — concise overview for plugin authors
Audience: Senior engineers
Tone: Practical, matter-of-fact
Target length: 1200 words

Key questions: spec, examples, best practices.
```

2) Run goresearch with your LLM endpoint configured (external OpenAI-compatible API):

```bash
export LLM_BASE_URL="https://your-llm.example.com/v1"
export LLM_MODEL="your/model-id"

goresearch \
  -input request.md \
  -output report.md \
  -llm.base "$LLM_BASE_URL" \
  -llm.model "$LLM_MODEL" \
  -llm.key "$LLM_API_KEY"
```

3) Open `report.md`. You should see a title and date, an executive summary, body sections with bracketed citations like `[3]`, a References list with URLs, an Evidence check appendix, and a reproducibility footer.

Tip: explore without calling the LLM first:

```bash
goresearch -input request.md -output report.md -dry-run -searx.url "$SEARX_URL"
```

## Configuration

You can configure via flags or environment variables.

Environment variables:
- `LLM_BASE_URL`: base URL for the OpenAI-compatible server (e.g., `http://localhost:1234/v1`)
- `LLM_MODEL`: model name
- `LLM_API_KEY`: API key for the server
- `SEARX_URL`: SearxNG base URL (e.g., `https://searx.example.com`)
- `SEARX_KEY` (optional): SearxNG API key
- `TOPIC_HASH` (optional): included for traceability in cache scoping

Primary flags (with defaults):
- `-input` (default: `request.md`): path to input Markdown research request
- `-output` (default: `report.md`): path for the final Markdown report
- `-searx.url`: SearxNG base URL
- `-searx.key`: SearxNG API key (optional)
- `-searx.ua`: Custom User-Agent for SearxNG requests (default identifies goresearch)
- `-search.file`: Path to a JSON file providing offline search results for a file-based provider
- `-llm.base`: OpenAI-compatible base URL
- `-llm.model`: model name
- `-llm.key`: API key
- `-max.sources` (default: 12): total sources cap
- `-max.perDomain` (default: 3): per-domain cap
- `-max.perSourceChars` (default: 12000): per-source character limit for excerpts
- `-min.snippetChars` (default: 0): minimum snippet chars to keep a search result
- `-lang` (default: empty): language hint, e.g. `en` or `fi`
- `-dry-run` (default: false): plan/select without calling the LLM
  - `-v` (default: false): verbose console output (progress). Detailed logs are controlled via `-log.level`.
  - `-log.level` (default: info): structured log level for the log file: trace|debug|info|warn|error|fatal|panic
  - `-log.file` (default: logs/goresearch.log): path to write structured JSON logs
  - `-debug-verbose` (default: false): allow logging raw chain-of-thought (CoT) for debugging Harmony/tool-call interplay. Off by default.
- `-cache.dir` (default: `.goresearch-cache`): cache directory
- `-cache.maxAge` (default: 0): purge cache entries older than this duration (e.g. `24h`, `7d`); 0 disables
- `-cache.clear` (default: false): clear entire cache before run
- `-cache.topicHash`: optional topic hash to scope cache (accepted for traceability)
- `-cache.strictPerms` (default: false): restrict cache at rest (0700 dirs, 0600 files)
- `-robots.overrideDomains` (default from env `ROBOTS_OVERRIDE_DOMAINS`): comma-separated domain allowlist to ignore robots.txt, requires `-robots.overrideConfirm`
- `-robots.overrideConfirm` (default: false): second confirmation flag required to activate robots override allowlist
 - `-domains.allow` (comma-separated): only allow these hosts/domains; subdomains included
 - `-domains.deny` (comma-separated): block these hosts/domains; takes precedence over allow
 - `-tools.enable` (default: false): enable the tool-orchestrated chat mode
 - `-tools.dryRun` (default: false): do not execute tools; append structured dry-run envelopes
 - `-tools.maxCalls` (default: 32): maximum number of tool calls per run
 - `-tools.maxWallClock` (default: 0): wall-clock cap for the tool loop (e.g., `30s`); 0 disables
 - `-tools.perToolTimeout` (default: 10s): per-tool execution timeout
 - `-tools.mode` (default: `harmony`): chat protocol mode: `harmony` or `legacy`
 - `-verify` / `-no-verify` (default: `-verify`): enable or disable the fact-check verification pass and Evidence check appendix

## Full CLI reference

For a comprehensive, auto-generated list of all flags and environment variables, see: [docs/cli-reference.md](docs/cli-reference.md).

## Run locally with Docker (optional)

Important: On Apple M2 virtual machines (including this development environment), Docker is not available due to nested virtualization limits. Use the non-Docker alternatives documented below (for example, Homebrew/venv SearxNG and a local LLM). On machines with Docker installed, you can run the full local stack.

### Prerequisites
- Docker Desktop with Compose v2 (or Docker Engine + `docker compose` CLI)
- Recommended: ≥4 CPUs and ≥8 GB RAM for the LLM service
- Network access to pull images on first run

goresearch is a CLI that you run on the host. Docker Compose is only used to provide optional dependencies.

Start SearxNG locally (optional for web search):

```bash
docker compose -f docker-compose.optional.yml up -d searxng
```

Optional services live in `docker-compose.optional.yml` and can be brought up as needed (TLS proxy only):

```bash
# TLS reverse proxy via Caddy — optional (needs optional file)
docker compose -f docker-compose.optional.yml --profile tls up -d caddy-tls
```

### Environment variables
Compose will read a local `.env` file when present and also respects exported shell variables. Useful settings:

- `LLM_BASE_URL`: base URL for your LLM server
- `LLM_MODEL`: model identifier known to your LLM server
- `LLM_API_KEY`: API key if your server requires one (not baked into images)
- `SEARX_URL`: internal URL for SearxNG (default `http://searxng:8080`)
- `SSL_VERIFY`: enable SSL certificate verification; set to `false` for self-signed certificates (default `true`)
- `APP_UID` / `APP_GID`: host user/group IDs to avoid permission issues on bind mounts (e.g., `APP_UID=$(id -u) APP_GID=$(id -g)` before `make up`)

### Health checks and readiness
- Services declare health checks: `searxng` probes `/status`.

Check health via:

```bash
docker compose -f docker-compose.optional.yml ps
docker compose -f docker-compose.optional.yml logs -f --tail=100
```

## Robots, opt-out, and politeness policy

... (content unchanged from original README)

## Tests

Run all tests:

```bash
go test ./...
```

... (remaining sections unchanged from original README)
