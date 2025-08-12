### Use case: “Enable HSTS correctly on Nginx (with preload) — decision brief”

**Goal**

Produce a one-page, evidence-backed executive summary that explains how to 
enable HTTP Strict Transport Security (HSTS) on Nginx for a production site, 
including `max-age`, `includeSubDomains`, and `preload` directives, validation 
steps, and risks/rollback guidance. Favor primary sources (RFC 6797, NGINX 
docs, browser preload docs). The output must be a well-formed Markdown report 
with citations and a references section.

**How to run (example command)**

```bash
goresearch \
  --q "How to enable HSTS correctly on Nginx (includeSubDomains & preload), validation steps, risks, rollback" \
  --lang en \
  --primary true \
  --verify true \
  --cache.maxAge=0
```

(Exercises CLI entrypoint, language hint, primary-source preference, verification pass, and cache controls.)&#x20;

**Why this is a good end-to-end scope**

* Hits every stage: search → fetch → extract → select → brief → synthesize → validate → verify.&#x20;
* Naturally pulls **primary** sources (RFC 6797, NGINX docs, MDN/Chrome docs), which lets you assert the selector’s “prefer primary sources” behavior.&#x20;
* Produces a concise, decision-ready brief that should meet your Markdown output contract (title/date/sections/refs), making validation deterministic.&#x20;

**Acceptance criteria (structure & artifacts)**

1. **Report structure passes validation**: title, date, logical headings, and a complete **References** list with full URLs are present (tool’s Markdown contract check).&#x20;
2. **Primary sources included**: references include at least two of: RFC 6797, official NGINX docs, MDN/Web security docs, or Chrome preload guidance; citations in-text point to these. (Exercises selection + primary-source preference.)&#x20;
3. **Verification appendix present**: an “Evidence”/verification appendix lists key claims (e.g., recommended `max-age`, preload caveats, validation via `curl`/security headers) with linked supporting sources and confidence.&#x20;
4. **Reproducibility footer present**: includes model name, base API URL, source count, and whether caching was active.&#x20;
5. **Embedded source manifest generated**: sidecar or embedded JSON lists canonical source URLs + SHA-256 digests.&#x20;
6. **Per-source failure isolation**: if one fetched page fails (simulate with a known bad URL via test harness), the run completes, citations reindex, and the bad source is skipped with a structured log.&#x20;
7. **Token-budget safety**: with ≥8 sources returned, proportional excerpt truncation keeps all sources represented without exceeding context. (Check logs for truncation event.)&#x20;

**Content expectations (spot-checks)**

* Executive summary states what HSTS is, the exact header to use in Nginx 
(`Strict-Transport-Security: max-age=...; includeSubDomains; preload`), 
verification steps (e.g., response header checks), and **risks/rollback** 
(e.g., subdomain lock-in, staging first, how to remove from preload). 
(Leverages the tool’s “decision-ready” report pattern and validation of section 
presence.) &#x20;

That one scenario gives you stable sources, exercises the full pipeline 
(including caching/flags, primary-source preference, output contract, manifest, 
and the fact-check pass), and yields clear, automatable assertions for 
CI.&#x20;

