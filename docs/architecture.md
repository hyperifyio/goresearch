# Architecture overview

This document provides a high-level overview of the end‑to‑end data flow and the primary package boundaries. The pipeline runs as a single CLI process and moves structured artifacts through discrete stages so each module can be reasoned about, tested, and swapped independently.

- Input brief (Markdown) → Plan queries and outline
- Search → Fetch → Extract → Select → Synthesize → Validate → Verify → Render
- Caching, robots/opt‑out compliance, and observability apply across stages

## Data flow (packages and external services)

```mermaid
flowchart LR
    subgraph CLI[CLI orchestrator]
        A[Brief (Markdown)] --> B[Planner\ninternal/app]
        B -->|queries| C[Search\ninternal/search]
        C -->|URLs| D[Fetch\ninternal/fetch]
        D -->|HTTP bodies| E[Extract\ninternal/extract]
        E -->|documents| F[Select\ninternal/select]
        F -->|budgeted excerpts| G[Synthesize\ninternal/synth]
        G -->|draft| H[Validate\ninternal/validate]
        H -->|validated doc| I[Verify\ninternal/verify]
        I -->|final doc + evidence| J[Render\ninternal/app]
    end

    C --- SEARX[(SearxNG / file provider)]
    D --- WEB[(Public web over HTTP/S)]
    G --- LLM[(OpenAI‑compatible LLM)]

    subgraph Infra[Cross‑cutting]
        K[HTTP cache]
        L[LLM cache]
        M[Robots / TDM opt‑out]
        N[Structured logging]
    end

    D <--> K
    G <--> L
    D --> M
    C --> N
    D --> N
    E --> N
    F --> N
    G --> N
    H --> N
    I --> N
```

## Notes on boundaries
- internal/search: provider‑agnostic interface (SearxNG by default; file‑based offline adapter supported).
- internal/fetch: polite HTTP client with robots/TDM checks, redirects, content‑type gating, and conditional revalidation.
- internal/extract: readability‑oriented HTML text extraction and normalization.
- internal/select: deduplication, diversity caps, and proportional budgeting.
- internal/synth: grounded Markdown synthesis via OpenAI‑compatible API.
- internal/validate: citation/link structure checks and normalization.
- internal/verify: short claim‑checking pass producing an evidence appendix.
- internal/app: orchestration, config, rendering, artifact/manifest writing.

See also: `README.md` → Architecture and design for a narrative description, and `docs/verification-and-manifest.md` for auditability details.
