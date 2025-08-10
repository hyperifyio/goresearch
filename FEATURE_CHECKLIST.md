* [x] Scope and goal — The tool reads a single Markdown file containing a natural-language research request, automatically searches the public web, extracts relevant content, and uses a local OpenAI-compatible LLM to produce a validated Markdown report with inline numbered citations, a references section, a limitations section, and an evidence-check appendix.

* [x] Single input brief parsing — The program ingests exactly one Markdown file and distills it into a brief that includes topic, optional audience and tone hints, and an optional target length if present, falling back to sensible defaults when fields are missing.

* [x] Planner with JSON output — The tool prompts the local LLM to return only structured JSON containing 6–10 diverse web search queries and a 5–8 heading outline; narrative text is explicitly disallowed in this planning step.

* [x] Planner robustness and fallbacks — If the planner’s JSON is malformed or missing, the tool recovers by generating deterministic fallback queries from the topic (for example, adding “specification”, “documentation”, “reference”, “tutorial” in the chosen language).

* [x] Search provider independence — Web search is performed via a configurable provider that is not OpenAI Search, with SearxNG as the default self-hosted option and an optional minimal HTML search adapter when allowed by terms.

* [x] Result aggregation and normalization — The program merges results from multiple queries, normalizes and canonicalizes URLs, trims tracking parameters where safe, and consolidates duplicates across queries.

* [x] Diversity-aware selection — The selector ranks candidates for topical diversity and domain diversity to avoid over-reliance on a single site or vendor perspective.

* [x] Source count and domain caps — The selector enforces a hard maximum number of sources and a per-domain cap to prevent citation monoculture while preserving breadth.

* [x] HTTP fetch timeouts — Each page fetch uses bounded timeouts to avoid hanging the run, with retry limited to transient network errors.

* [x] Polite user agent and rate limiting — Requests include a descriptive user agent string and observe configurable concurrency limits to reduce load on target sites.

* [x] Conditional revalidation — The fetcher records and honors ETag and Last-Modified headers to revalidate cached pages efficiently on subsequent runs.

* [x] Redirect handling — The fetcher follows redirects within a modest hop limit, rejecting redirect loops and non-HTTP schemes.

* [x] Content-type gating — Only HTML and XHTML responses are processed in the baseline; binary formats are declined to keep extraction predictable.

* [x] HTML extraction focus — The extractor prefers semantic containers such as main and article and falls back to body, preserving headings, paragraphs, list items, pre/code blocks, and other content-bearing elements. (implemented with tests)

* [x] Boilerplate reduction — Navigation, cookie banners, and footer chrome are reduced using simple content-density heuristics so that the extracted text concentrates on primary content. (cookie/consent banner heuristics implemented with tests)

* [x] Text normalization — Extracted text is normalized to Unicode, whitespace is collapsed, and near-duplicate lines are removed to improve token efficiency.

* [x] Low-signal filtering — Sources with too little meaningful text are discarded early to avoid wasting context on pages without substantive content.

* [x] Token budget estimation — The tool estimates prompt size from character counts and model characteristics to keep the combined system message, user message, and excerpts within the model’s context window. (added estimator + dry-run reporting)

* [x] Proportional truncation — When total extracts exceed budget, each document’s excerpt is trimmed proportionally rather than dropping entire sources, unless redundancy is detected. (implemented with tests)

* [x] Preference for primary sources — When topics are technical formats or specs, the selector favors primary documentation and authoritative references over secondary commentary.

* [x] Synthesis role and guardrails — The synthesis system message defines the model as a careful technical writer who uses only provided sources for facts, cites precisely, and states uncertainty where evidence is insufficient.

* [x] Structured document request — The user message for synthesis includes the brief, the outline (if available), target length, a numbered list of sources with titles and URLs, and per-source excerpts, and requests a single cohesive Markdown document with title, date, executive summary, body, risks and limitations, and references.

* [x] Inline citation format — Factual statements must be cited inline using bracketed numbers such as [n] that map to the numbered references list, with multiple citations allowed like [2][5].

* [x] No invented sources — The synthesis prompt forbids creating or altering sources and makes clear that only the enumerated sources may be cited.

* [x] Conservative generation settings — Low temperature and concise style are used to reduce embellishment and improve determinism of the produced report.

* [x] Citation validation — After synthesis, the tool validates that every cited index refers to an actual references entry and flags any out-of-range or broken citations.

* [x] Reference list completeness — The final references section includes both page titles and full URLs for each numbered source so readers can resolve citations directly. (validated in code with tests)

* [x] Fact-check verification pass — A second short LLM pass extracts key claims from the document, maps each to minimal supporting source indices, assigns confidence levels, and marks any claims that are weakly supported or unsupported. (implemented with LLM + deterministic fallback, evidence appendix appended)

* [x] Evidence map appendix — The verification result is appended as a compact evidence map that narratively summarizes support strength and lists claims with their cited indices and confidence without ornate formatting.

* [x] Graceful verification failure — If the verification call fails or returns invalid data, the tool omits the appendix, emits a warning, and preserves the main report.

* [x] Markdown output contract — The output is valid, renderer-friendly Markdown that avoids decorative flourishes and maintains a sensible heading hierarchy matching the outline. (validator + tests added)

* [x] Reproducibility footer — The document ends with a short footer that records the model name, the LLM base URL, the number of sources used, and whether HTTP and LLM caching were active.

* [x] Language hint propagation — A language hint can be provided; planner queries incorporate the language, the synthesizer writes in that language, and the selector prefers sources whose detected language matches when diversity allows. (implemented with tests)

* [x] Language tolerance — The system does not hard-filter by language so that authoritative English sources can still be used when researching in another language if necessary.

* [x] Configuration by flags and environment — The executable reads configuration from command-line flags and environment variables for LLM endpoint, model, key, source caps, truncation limits, timeouts, language, output path, and search provider settings.

* [x] Dry-run mode — A diagnostic mode prints the planned queries and selected URLs without calling the LLM, aiding transparency and debugging.

* [x] HTTP response caching — A local cache stores fetched page bodies keyed by canonical URL plus salient request header hashes for efficient re-use and audit.

* [x] LLM response caching — Calls to the planner and synthesizer can be cached by a normalized prompt digest and model name to speed up iterative runs.

* [x] Cache invalidation controls — The cache can be invalidated by age, by explicit flags, or by topic hash to ensure fresh data when needed.

* [x] Embedded manifest — The final report embeds or ships with a compact manifest listing canonical URLs and their content digests so others can audit exactly what was read.

* [x] Structured logging — The tool logs structured events with timestamps and levels and records planned queries, chosen sources, fetch durations, extract sizes, token estimates, and LLM latency without exposing secrets.

* [x] Verbose prompt logging — An opt-in verbose mode can print the exact planner and synthesizer messages with optional redaction of long excerpts to aid troubleshooting.

* [x] Per-source failure isolation — Network and parse errors are isolated per URL so that one failing site does not abort the whole run.

* [x] Planner failure recovery — If the planner cannot produce parseable output, deterministic fallback queries are generated to keep the pipeline progressing. (tests: planner fallback + app facade)

* [x] Synthesis retry policy — Transient LLM errors during synthesis trigger a single short backoff retry before failing the run. (implemented with test-injectable sleep and flaky client test)

* [x] Exit code policy — The program exits nonzero only when no usable sources are found or the LLM returns no substantive body text; otherwise it completes with warnings as needed. (implemented with sentinel errors and CLI mapping; tests added)

* [x] Robots and crawling etiquette — The fetcher respects robots meta where applicable, avoids crawling behind search result pages, and keeps request patterns polite. (search-results filtering added in selector with tests; meta/robots.txt support pending in later items)

* [x] Public web only — The tool targets public pages and does not authenticate to private services; outbound connections are limited to the configured search endpoint, fetched sites, and the local LLM endpoint. (blocks localhost/private IPs and credentials-in-URL in fetcher; tests added)

* [x] Optional cache at rest protection — The cache directory supports optional encryption or restricted permissions when environments require at-rest protection. (restricted permissions added via `-cache.strictPerms`)

* [x] Unit test coverage — Deterministic unit tests cover URL normalization, HTML extraction, deduplication, token budgeting, and citation validation using fixed fixtures.

* [x] Integration test harness — Integration tests run against a stub LLM that returns canned JSON and Markdown and against recorded HTTP fixtures to validate the pipeline deterministically. (added `internal/app/integration_llm_test.go`)

* [x] Golden output comparisons — Generated reports are compared against golden files with allowances for timestamps and benign whitespace differences to detect regressions.

* [x] Verification test cases — Synthetic documents include properly cited and deliberately uncited claims to confirm that the verification pass flags unsupported statements. (tests added in `internal/verify/verify_test.go`)

* [x] Concurrent fetch limits — Fetch and extract stages run concurrently up to a configurable limit to balance performance and site politeness.

* [x] Context budgeting heuristics — Input sizing uses conservative character-to-token multipliers and headroom to avoid overrunning the model’s context window. (implemented: headroom tokens, truncation and estimator updated; tests added)

* [x] Single-pass synthesis — The baseline uses a single synthesis pass for simplicity and predictability; streaming output is deferred to future work.

* [x] Adapter-based extensibility — Search and extraction modules are built behind narrow interfaces so providers and readability tactics can be swapped without touching the rest of the pipeline.

* [x] Prompt profile flexibility — Synthesis and verification prompts are externally configurable so teams can tune style, tone, and strictness without code changes.

* [x] Optional PDF support switch — PDF ingestion can be added as an optional, off-by-default feature guarded by a command flag to control binary parsing risk.

* [x] Known limitations disclosure — The design acknowledges dependence on local model capabilities, uneven public coverage for some topics, imperfect boilerplate removal, and the need for human review for high-stakes accuracy.

* [x] Operational run clarity — The end-to-end run is deterministic and auditable: input brief to planner to search to extraction to selection to synthesis to validation to verification to rendering, with each stage’s artifacts traceable via logs and the embedded manifest.

* [x] Robots.txt fetch and cache — For each host, fetch /robots.txt once per run with a clear User-Agent, honor ETag and Last-Modified for revalidation, cache parsed rules per host with an expiry, and reuse across requests. (implemented `internal/robots` with HTTP+disk cache, in-memory expiry, and tests)

* [x] Robots.txt parser with UA precedence — Evaluate rules first for the explicit tool User-Agent, then fall back to the wildcard agent; implement longest-path match and Allow vs Disallow precedence, including * wildcards and $ end anchors. (implemented evaluator + tests)

* [x] Crawl-delay compliance — If a Crawl-delay is present for the matched agent, enforce it with a per-host scheduler that spaces requests accordingly; integrate with existing concurrency limits.

* [x] X-Robots-Tag handling — Parse X-Robots-Tag headers on HTTP responses and treat opt-out signals relevant to text-and-data-mining (e.g., noai, notrain) as hard denials for reuse; record decisions in logs and manifest. (implemented: denial in fetch with tests; manifest logging pending in later item)

* [x] HTML meta robots handling — Parse page-level <meta name="robots"> and <meta name="googlebot"> directives; treat noai/notrain equivalents as hard denials, and record the matched directive and scope. (implemented in fetch with tests)

* [x] TDM opt-out recognition — Treat machine-readable text-and-data-mining reservation signals placed “in or around” the content as a fetch/use denial for research synthesis; prefer conservative interpretation when ambiguous. (implemented: HTTP Link rel="tdm-reservation" and HTML <link rel="tdm-reservation"> with tests)

* [x] Missing robots policy — When /robots.txt returns 404, proceed as allowed; when it returns 401/403/5xx or times out, treat as temporarily disallowed for that host and retry only on the next run or after cache expiry. (implemented with tests in `internal/robots/robots_test.go` and logic in `internal/robots/robots.go`)

* [x] Per-host allowlist override — Support an explicit, documented override flag that enables ignoring robots.txt for a bounded allowlist of domains (e.g., internal mirrors); emit a prominent warning and require a second confirmation flag.

* [x] Deny-on-disallow enforcement — Before each fetch, evaluate the URL path against cached rules; skip disallowed URLs, avoid partial reads, and short-circuit redirects that would land on disallowed paths.

* [x] Robots decision logging — For every skipped URL, log the host, matched directive, agent section used, and rule pattern; include a “skipped due to robots” section in the run manifest. (implemented with detailed decisions + manifest section and tests)

* [x] Tests for robots evaluation — Add unit tests covering UA-specific vs wildcard sections, longest-match precedence, Allow overriding Disallow, wildcard and anchor behavior, Crawl-delay enforcement, and header/meta opt-outs.

* [x] Documentation of policy — Document default-on compliance, the override mechanism, and the exact decision policy for 404 vs 401/403/5xx outcomes, making clear that the tool is intended for public web pages and polite use. (README section added with flags and behavior)

* [x] Tool protocol adapter — Add an OpenAI-compatible “tools/functions” encoder and response parser (name, description, JSON Schema args; detect and read `tool_calls` in assistant messages). 
* [x] Harmony response handling — Support Harmony-style channels (analysis/commentary/final) so the loop can safely ignore raw CoT except for explicit tool calls and final answers.

* [x] CoT redaction policy — Do not log or surface raw chain-of-thought by default; expose behind a debug flag since gpt-oss may interleave tool calls within CoT. (added `ContentForLogging`, `DebugVerbose` flag wired to synthesizer)

* [x] Tool registry & versioning — Central registry mapping stable tool names to internal functions; include semantic version and capability meta for reproducibility.

* [x] Orchestration loop — Iterate: send messages + tool schema, execute any returned tool calls, append tool results as `tool` messages, stop on final assistant text. (implemented in `internal/llmtools/orchestrator.go` with tests)

* [ ] Loop guards — Enforce max tool-calls per run, wall-clock budget, and per-tool timeouts to prevent runaway loops.

* [ ] Minimal tool surface — Expose just what the model needs: `web_search`, `fetch_url`, `extract_main_text`, and `load_cached_excerpt` (IDs) to start; expand later.

* [ ] Result size budgeting — Cap tool result payloads (chars/tokens); auto-summarize or return cache IDs for large blobs instead of inlining full text.

* [ ] Tool-output schemas — Define strict JSON result shapes per tool (including typed errors) and validate before feeding back to the model.

* [ ] Error recovery for tools — On invalid args or failures, return structured error objects; allow the model to retry with corrected parameters.

* [ ] Deterministic IDs — Assign stable content IDs/digests for fetched pages and extracts so the model can request slices by ID instead of re-pulling text.

* [ ] Policy enforcement in tools — Apply existing robots/opt-out/host politeness rules inside tool execution so the model cannot bypass them (deny-on-disallow).

* [ ] Domain allow/deny lists — Centralized allowlist/denylist evaluated before any networked tool runs; log blocked attempts.

* [ ] Safety redaction — Strip secrets, cookies, and tracking params from tool outputs; scrub headers before echoing anything into the transcript.

* [ ] Structured tracing — Log every tool call with tool name, args hash, duration, byte counts, and outcome; correlate to the final report for auditability.

* [ ] Cache-aware tools — Tools consult HTTP/LLM caches; add a per-tool “cache only / revalidate / bypass” flag wired to your existing caching layer.

* [ ] Dry-run for tools — A mode that prints intended tool calls (with redacted args) without executing them; useful for debugging prompt-tool interplay.

* [ ] Fallback path — If the model doesn’t call tools (or the adapter is disabled), fall back to your current planner→search→synthesis pipeline.

* [ ] Prompt affordances — System/developer messages that document each tool’s contract, limits, and when to use it; keep this text concise to save tokens.

* [ ] Token/context budgeting for tool chat — Heuristics to prune earlier loop turns and compress older tool outputs so the running conversation stays within context.

* [ ] Tests: tool loop — Deterministic integration tests with a stub model that requests specific tool calls in order; assert call sequencing and final answer assembly.

* [ ] Tests: schema & fuzz — Unit tests that validate tool arg/result schemas and fuzz malformed inputs to ensure graceful errors.

* [ ] Config flags — CLI/env switches to enable tools, set loop caps/time budgets, and toggle Harmony/legacy function-calling modes.

* [ ] Manifest extensions — Record the ordered tool-call transcript (names, args hash, result digests) in the embedded manifest for third-party audit.

* [ ] Docker Compose local stack — Provide docker-compose.yml with services: research-tool, searxng (default search), llm-openai (local OpenAI-compatible LLM server), and stub-llm (for tests). Use a dedicated bridge network, named volumes for http\_cache, llm\_cache, and reports, and Compose profiles: dev (tool+searxng+llm), test (tool+stub-llm), and offline (tool only, cache-only mode).

* [ ] Research tool container — Add a minimal Dockerfile for the CLI with a non-root user, pinned base image, labels (org.opencontainers), build args for version/commit, and an entrypoint that reads config from env/flags. Mount ./reports and ./cache as writable volumes. Include healthcheck that runs a quick “--dry-run” and exits 0 on success.

* [ ] OpenAI-compatible LLM server container — Include a generic llm service (image pinned by digest) exposing an OpenAI-compatible /v1 API. Allow model path/ID and quantization via env, mount a models volume, and add a readiness healthcheck on /v1/models. Make the tool depend\_on this service becoming healthy.

* [ ] SearxNG container — Add a searxng service (image pinned by digest) with mounted settings.yml, custom User-Agent, reduced concurrency, and safe rate limits. Expose internal URL to the tool only via the Compose network (no public port by default). Healthcheck /status page.
    - Also provide non-Docker options: Homebrew and Python venv setup; add file-based provider for offline/no third-party dependency operation.

* [ ] Model weights volume & bootstrap — Define a models named volume and an optional one-shot init container to fetch or copy local weights into the volume, with checksum verification and clear failure on mismatch.

* [ ] Environment & secrets handling — Support a .env file and a committed .env.example documenting required variables (LLM\_BASE\_URL, LLM\_MODEL, SEARXNG\_URL, CACHE\_DIR, LANGUAGE, SOURCE\_CAPS). Ensure secrets are not baked into images; pass keys only via env or mounted files.

* [ ] Health-gated startup — Use depends\_on with condition: service\_healthy so the tool starts only after llm-openai and searxng are ready. Provide a make wait target that polls health for local troubleshooting.

* [ ] Resource limits — Set conservative cpu/memory limits and reservations per service; document how to override (e.g., COMPOSE\_PROFILES=dev LLM\_MEMORY\_GB=8). Ensure the tool fails gracefully when limits are hit.

* [ ] Non-root volumes & permissions — Create volumes with matching UID\:GID for the app user in containers; provide a helper script/compose override to chown existing host directories to avoid permission errors.

* [ ] Logs & artifacts mapping — Map structured JSON logs to ./logs and final Markdown reports to ./reports. Ensure timestamps are in UTC and file names are stable (topic hash or timestamp).

* [ ] Reproducible images — Pin all service images by digest, add SBOM export at build (BuildKit attestations), and label images with vcs-ref and build-date for traceability.

* [ ] Make targets for DX — Add make up, make down, make logs, make rebuild, make test (uses test profile + stub-llm), and make clean (prunes volumes for caches). Document one-liners in README.

* [ ] CI compose smoke test — GitHub Actions workflow that builds the tool image, brings up the test profile (tool+stub-llm), runs a canned brief, asserts a report file exists, and uploads it as an artifact.

* [ ] Offline/airgapped profile — Provide an offline Compose profile that disables searxng and runs the tool in cache-only mode (both HTTP and LLM caches), failing fast if a cache miss occurs.

* [ ] Local TLS (optional) — Optional caddy/nginx reverse-proxy service for local HTTPS termination to llm and searxng with self-signed certs; disabled by default and isolated to the Compose network.

* [ ] Network isolation — Use a private Compose network; do not publish ports by default. The tool reaches only llm-openai and searxng by service name. Document an override file to expose ports when needed.

* [ ] Integration test runner container — Add a disposable test-runner service that executes deterministic integration tests against stub-llm and recorded HTTP fixtures, producing JUnit/HTML results under ./reports/tests.

* [ ] Multi-arch builds — Configure buildx targets for linux/amd64 and linux/arm64, with QEMU emulation in CI, and publish local artifacts for both arches for developers on Intel/Apple Silicon.

* [ ] Cache at-rest option — If cache encryption is enabled in the app, wire it to a dedicated cache volume and expose a COMPOSE profile/env toggle to activate encryption or restricted permissions at runtime.

* [ ] Documentation snippet — Update README with a “Run locally with Docker” section covering prerequisites, one-line dev start, profiles, environment variables, health checks, and common troubleshooting steps.

* [ ] Single-file config support — Add support for goresearch.yaml|json with env-var overrides, schema validation, and `goresearch init` to scaffold a config and `.env.example`.&#x20;

* [ ] Quick Start in README — One copy-paste command with expected output, plus a “hello research” example brief and result.&#x20;

* [ ] Full flag & config reference — Auto-generate a comprehensive CLI/options page and link it from README.&#x20;

* [ ] Feature guides for verification & manifest — Explain how the evidence appendix is produced/read and how to use the embedded/sidecar manifest for audit.&#x20;

* [ ] Architecture overview & diagram — Document Search → Fetch → Extract → Select → Synthesize → Validate → Verify data flow with package boundaries.&#x20;

* [ ] Quiet default + log levels — Make concise progress output the default; route detailed structured logs to file and gate with `--log-level`. Document how to enable verbose.&#x20;

* [ ] Cache size limits & eviction — Add max bytes/count with LRU eviction for HTTP and LLM caches, alongside existing age-based invalidation.&#x20;

* [ ] Verification toggle — Add `--verify/--no-verify` to explicitly enable/disable the fact-check pass and appendix.&#x20;

* [ ] Artifacts bundle export — Persist planner JSON, selected URLs, extracts, final report, manifest, and evidence appendix under `./reports/<topic>/` and optional tarball with digests for offline audit.&#x20;

* [ ] Graceful cancel & resume — On SIGINT/SIGTERM, write partial artifacts and allow fast resume from cache on next run to keep UX “dead simple.”&#x20;

* [ ] CONTRIBUTING.md + templates — Add contribution guide (coding style, running tests, Cursor rules, commit semantics) and PR/issue templates.&#x20;

* [ ] Release packaging — Use GoReleaser to ship macOS/Linux/Windows binaries with version/commit info, checksums, and SBOM; publish via CI.&#x20;

* [ ] Benchmarks — Add Go benchmarks for fetch, extract, selection, and token budgeting to quantify the impact of concurrency/politeness settings.&#x20;

* [ ] Static analysis & pre-commit — Enforce `go fmt`, `go vet`, `staticcheck`, and pre-commit hooks; wire into CI next to existing tests.&#x20;

* [ ] Troubleshooting & FAQ — Document common failures (cache, robots/opt-out denials, LLM endpoint issues) and how to raise verbosity to debug.&#x20;

* [ ] LLM backend interface — Extract a provider interface so different OpenAI-compatible or local backends can be swapped without touching core logic.&#x20;

* [ ] Report-type templates & section profiles — selectable IMRaD, decision/tech report, and literature-review profiles that enforce the right sections per type.&#x20;

* [ ] Executive Summary guardrails — length target (\~150–250 words) and content checks (motivation, methods, key results, recommendations).&#x20;

* [ ] Title quality check — enforce ≤12 words, descriptive keywords, and no unexplained acronyms/jargon.&#x20;

* [ ] Heading audit — require descriptive “mini-title” headings, consistent hierarchy/parallel phrasing; optional auto-numbering for long reports.&#x20;

* [ ] Plain-language & readability lint — active-voice preference, acronym defined on first use, average sentence length \~15–20 words, target reading level with metrics.&#x20;

* [ ] Audience fit check — per-brief audience/tone settings and a pass that flags jargon or sections mismatched to the intended reader.&#x20;

* [ ] Glossary & acronym list — auto-extract key terms and acronyms; add optional “Glossary” appendix.&#x20;

* [ ] Visuals QA — numbered figures/tables with captions, required in-text references (“See Fig. X”), and alt text; verify placement near discussion.&#x20;

* [ ] Table of Contents for long reports — auto-generate ToC when document exceeds a size threshold.&#x20;

* [ ] References enrichment — resolve/insert DOIs where available, add stable URLs and “Accessed on” dates for web sources; completeness validator.&#x20;

* [ ] Reference quality/mix validator — configurable policy to prefer peer-reviewed/standards, ensure recency where appropriate, and prevent over-reliance on a few sources.&#x20;

* [ ] Counter-evidence search step — inject queries like “limitations of X / contrary findings”, and require a short “Alternatives & conflicting evidence” subsection.&#x20;

* [ ] Reporting-guideline profiles — optional PRISMA/CONSORT/EQUATOR compliance checks tied to report type; for reviews, emit a simple PRISMA-style inclusion/exclusion table/diagram.&#x20;

* [ ] Proofreading pass — grammar/spell/consistency check (units, terminology, capitalization) before final render.&#x20;

* [ ] “Ready for distribution” checks — validate metadata (author/date/version), link targets, and optionally produce a PDF with working hyperlinks.&#x20;

* [ ] Accessibility checks — heading order correctness and “no color-only meaning” warnings; require alt text for any images.&#x20;

* [ ] Recommendations section (optional) — generate when the brief expects decisions/actions, separate from Conclusions.&#x20;

* [ ] Appendix management — auto-label Appendices A/B/C…, ensure each is referenced from the body.&#x20;
