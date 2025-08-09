# goreseach

Scope and goal. The tool reads a single Markdown file that describes a research 
request in natural language, for example “Detailed documentation about Cursor 
MDC format including examples”. It automatically plans a web research strategy, 
discovers and fetches relevant public web pages, extracts readable text, and 
asks a local OpenAI-compatible model to synthesize a validated Markdown 
document. The output is a self-contained report with a predictable structure, 
numbered inline citations, a references section with working URLs, and a short 
limitations and evidence-check appendix. The system must operate using only the 
standard chat completions style API that OpenAI-compatible servers expose. No 
OpenAI Search API or proprietary search features are used. The primary LLM 
target is a local OSS GPT endpoint that implements the chat completions 
interface.

High-level architecture. The application is a single binary command-line 
program composed of loosely coupled modules: request parsing, planning, search, 
fetch and extract, selection and deduplication, token budgeting, synthesis, 
verification, rendering, caching, and observability. Each module exposes a 
narrow interface so implementations can be swapped without affecting the rest. 
For example, the search module can point to a self-hosted meta-search engine 
such as SearxNG, or to a minimal HTML search scraper, without changing how 
results flow to the selector.

End-to-end flow. The run begins by loading the input Markdown file and reducing 
it to a concise research brief composed of topic, optional audience and tone 
hints if present, and an optional maximum word count if the file contains an 
explicit constraint. The planner invokes the local LLM with a system 
instruction that forbids narrative output and requests only structured data. 
The single user message contains the brief text and asks the model to propose a 
small set of precise web queries that together cover the topic comprehensively 
and to draft an outline with section headings. The tool parses the LLM’s 
response, tolerates minor deviations such as extra whitespace, and falls back 
to simple heuristic queries based on the topic if the response cannot be 
parsed. The search module executes the queries against a configured provider. 
The default provider is a self-hosted SearxNG instance over HTTP with an API 
key or IP allowlist, because this keeps the dependency controllable while 
avoiding vendor lock-in. As an optional alternative for development 
environments, the module can use a generic web search HTML endpoint if allowed 
by that site’s terms. The program aggregates results, normalizes URLs, removes 
duplicates, and ranks candidates by textual diversity and domain diversity. The 
selector enforces a maximum overall source count and a per-domain cap to avoid 
citation monoculture.

Content fetching and extraction. The fetcher issues HTTP GET requests with a 
configurable timeout, a descriptive user agent string, and polite rate 
limiting. It records ETag and Last-Modified values to support conditional 
revalidation on repeat runs. It follows redirects within a modest hop limit and 
rejects non-HTTP and data URLs. The extractor only processes HTML and XHTML 
content types for the baseline version and declines to parse binary formats. It 
constructs a lightweight DOM and extracts text from semantic containers such as 
main and article if present, else body, and keeps structural elements like 
headings, paragraphs, list items, and code blocks. Common navigation chrome, 
cookie banners, and footer boilerplate are reduced using simple density 
heuristics that prioritize longer, content-rich blocks. The extracted text is 
normalized to Unicode, whitespace is collapsed, and near-duplicate lines are 
removed. Each document carries its canonical URL, detected title, and extracted 
text. Documents that produce too little meaningful text are discarded early.

Source selection and budgeting. After extraction, the tool applies a token 
budget. It caps the number of documents and truncates each document’s text to a 
maximum number of characters that together fit within the model’s input limit 
once prompts and system messages are accounted for. If the combined extracts 
still exceed the target, the tool reduces each extract proportionally rather 
than dropping entire sources, unless a source is clearly redundant. The 
selection prefers diversity across domains and viewpoints when the topic 
invites it and prefers primary documentation when the topic is a software 
specification or format.

Synthesis prompt and grounding. The synthesizer crafts two messages. The system 
message defines the assistant role as a careful technical writer and explicitly 
requires the use of only the provided sources for factual claims. It explains 
the expected document structure, prescribes numbered inline citations using 
square brackets that map to the numbered source list, and instructs the model 
to state uncertainty when evidence is insufficient. The user message provides 
the brief, the required or suggested section headings from the outline if 
available, a target length, the numbered list of sources with titles and URLs, 
and the excerpts for each source. The tool asks for a single Markdown document 
that contains a title and run date, an executive summary written as short lines 
rather than decorative bullets, a main analysis section organized by headings, 
a risks and limitations section, and finally a references section where items 
are numbered to match the inline citations. Temperature is kept low to preserve 
determinism and reduce embellishment. The model is not asked to reveal hidden 
chain of thought and is prohibited from inventing sources.

Validation and claim checking. Once the draft is produced, the tool validates 
citation syntax and ensures that every bracketed citation number refers to an 
existing entry in the references section. Any broken or out-of-range citation 
markers are flagged and either removed or mapped to a best effort reference. 
The tool then engages the model in a second, short fact-checking pass. The 
system role now declares a fact-checker, and the user message contains the 
generated document and the numbered source list. The model is asked to extract 
a set of key claims, map each to the minimal set of supporting source indices, 
assign a confidence level, and flag any claims that are weakly supported or 
unsupported by the supplied sources. The result is turned into an evidence map 
that is appended to the output. If the verification call fails or produces 
invalid data, the program proceeds without the appendix and records a warning.

Output contract. The output is always valid Markdown that renders cleanly in 
GitHub and static site generators. The heading hierarchy follows the outline 
but avoids decorative flourishes. Inline citations are plain bracketed numbers 
that correspond to the final references section. The references list includes 
both page titles and full URLs so the document remains useful offline. The 
evidence map is a simple narrative paragraph describing which claims are well 
supported and which require caution, accompanied by a compact line-based 
listing of claims and their cited indices without complex formatting. The 
generated document includes a reproducibility footer that records the model 
name, the base URL of the LLM endpoint, the number of sources ingested, and 
whether caching was in effect.

Configuration and runtime controls. The program is configured through 
environment variables and flags so it can be scripted. The LLM base URL, model 
name, and API key are required and point to a local OpenAI-compatible service 
such as an OSS GPT runtime. The maximum number of sources, per-domain cap, 
per-source character limit, timeouts, language hint, and output path are 
adjustable. Search provider configuration is explicit and kept separate from 
LLM settings. A dry-run switch prints the planned queries and selected URLs 
without calling the model to assist in debugging and to provide transparency.

Caching and reproducibility. A local cache directory stores fetched page bodies 
keyed by URL plus a content hash of important request headers, as well as LLM 
request-response pairs keyed by a normalized prompt digest and model name. On 
subsequent runs, the tool reuses valid cache entries or revalidates HTTP 
documents using conditional requests. The cache can be invalidated by age, by 
explicit flags, or by topic hash to support iterative research while preserving 
determinism. The final report embeds a compact manifest that lists the 
canonical URLs and their content digests used in synthesis so downstream users 
can audit what was read.

Observability and logging. The tool emits structured logs to standard error 
with timestamps and levels. It logs planned queries, chosen sources, fetch 
durations, extraction sizes, token budget estimates, and LLM latency. Sensitive 
values such as API keys are never logged. A verbose mode prints the exact 
system and user messages sent to the model, but only when explicitly enabled, 
and can redact long excerpts to avoid clutter.

Error handling and resilience. Network errors, search outages, and extraction 
failures are isolated per source, allowing the run to proceed with remaining 
documents. If the planner fails to return parseable queries, the system 
composes a small set of deterministic fallbacks by combining the topic with 
intent words such as specification, documentation, tutorial, and reference in 
the configured language. If synthesis fails with a transient LLM error, a 
single retry with a short backoff is attempted. The program returns a nonzero 
exit code only when no usable sources are found or when the LLM cannot produce 
any body text.

Security, compliance, and politeness. The fetcher respects robots meta tags and 
avoids crawling behind search result pages. It rate limits concurrent requests 
and sets a clear user agent that identifies the tool and a contact URL. The 
program is designed for public web pages and does not authenticate to private 
services. It does not exfiltrate secrets because the only outbound destinations 
are the configured search endpoint, the fetched sites, and the local LLM 
endpoint. The cache directory can be encrypted at rest if the runtime 
environment requires it.

Language and localization. The planner and synthesizer accept a language hint. 
When provided, queries include that language and the synthesizer writes the 
report in the same language. The selector does not hard filter by language, 
because some authoritative sources may be in English even when the requested 
language is Finnish; instead, the tool prefers sources whose page language 
matches the hint when diversity allows.

Testing strategy. The project includes deterministic unit tests for URL 
normalization, HTML extraction, deduplication, token budgeting, and citation 
validation. It includes integration tests that run against a stub LLM server 
which returns canned JSON for planning and a canned Markdown for synthesis to 
verify the control flow without needing a real model. It includes record and 
replay fixtures for representative web pages so extraction logic is stable 
across versions. Golden outputs are compared with tolerances for timestamps and 
minor whitespace. The verification pass is tested with synthetic documents that 
contain both properly cited and deliberately uncited claims to ensure 
unsupported statements are flagged.

Performance profile. The fetch and extract stage runs concurrently up to a 
configurable limit to avoid overwhelming a site or the network. Token budgeting 
is computed from measured character counts with a conservative multiplier so 
prompts fit within the local model’s context. The synthesizer runs in a single 
pass for simplicity, which is sufficient for short to medium reports. Streaming 
output can be added later, but the baseline waits for full completion to 
simplify validation.

Extensibility. The search module can be extended with additional adapters 
without touching the rest of the program as long as each adapter yields a list 
of title, URL, and short snippet. The extractor can incorporate a proper 
readability algorithm or integrate a site-specific ruleset for popular 
documentation sites. The synthesis prompts can be swapped by configuration to 
tune style and citation strictness per project. A future extension can add 
optional PDF ingestion with a small text extractor when the site hosts 
authoritative PDFs, guarded by a per-run switch so users can control binary 
parsing.

Constraints and limitations. The quality of synthesis depends on the local 
model’s instruction following and context window. Some topics may be poorly 
covered by publicly accessible pages or hidden behind paywalls. HTML extraction 
cannot perfectly remove boilerplate on all sites, which may reduce token 
efficiency. The tool does not guarantee legal or medical accuracy and should be 
treated as an assistant for drafting rather than an oracle. The verification 
appendix increases confidence but is not a substitute for human review.

Operational summary. The user provides a single Markdown file containing the 
research request and runs the program with the LLM endpoint and search 
configuration. The planner asks the local model for queries and an outline. The 
searcher retrieves candidates and the fetcher extracts text. The selector 
applies diversity and budget rules. The synthesizer produces a clean Markdown 
document grounded in the extracts with numbered citations. The validator fixes 
citation issues and the verifier builds an evidence map. The renderer writes 
the final document to disk, along with a manifest that describes models and 
sources so the result is auditable and reproducible.
