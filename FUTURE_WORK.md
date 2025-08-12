
-* [ ] Quick Start in README — One copy-paste command with expected output, plus a “hello research” example brief and result.&#x20;

* [ ] Environment & secrets handling — Support a .env file and a committed .env.example documenting required variables (LLM\_BASE\_URL, LLM\_MODEL, SEARXNG\_URL, CACHE\_DIR, LANGUAGE, SOURCE\_CAPS). Ensure secrets are not baked into images; pass keys only via env or mounted files.

* [ ] Resource limits — Set conservative cpu/memory limits and reservations per service; document how to override (e.g., COMPOSE\_PROFILES=dev LLM\_MEMORY\_GB=8). Ensure the tool fails gracefully when limits are hit.

* [ ] References enrichment — resolve/insert DOIs where available, add stable URLs and “Accessed on” dates for web sources; completeness validator.&#x20;

-* [ ] Proofreading pass — grammar/spell/consistency check (units, terminology, capitalization) before final render.&#x20;

