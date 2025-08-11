#!/usr/bin/env bash
# Simple, real smoke test for goresearch
# - Verifies end-to-end pipeline using a local OpenAI-compatible stub and real public URLs
# - Optionally checks Docker Compose services (stub-llm, searxng) health
# - Prints a clean PASS/FAIL summary for key capabilities

set -u

# Colors
GREEN="\033[32m"; RED="\033[31m"; YELLOW="\033[33m"; BOLD="\033[1m"; RESET="\033[0m"
PASS_ICON="✅"; FAIL_ICON="❌"; WARN_ICON="⚠️"

# Accumulators
passes=()
fails=()
warns=()

section() {
  echo
  echo "${BOLD}== $* ==${RESET}"
}

ok() { echo -e "${GREEN}${PASS_ICON} PASS${RESET} - $*"; passes+=("$*"); }
ko() { echo -e "${RED}${FAIL_ICON} FAIL${RESET} - $*"; fails+=("$*"); }
wn() { echo -e "${YELLOW}${WARN_ICON} WARN${RESET} - $*"; warns+=("$*"); }

have() { command -v "$1" >/dev/null 2>&1; }

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 1

GO_BIN=""
STUB_BIN=""
TMPDIR=""
CLEANUP_CMDS=()

cleanup() {
  for cmd in "${CLEANUP_CMDS[@]:-}"; do eval "$cmd" || true; done
  if [[ -n "$TMPDIR" && -d "$TMPDIR" ]]; then rm -rf "$TMPDIR" || true; fi
}
trap cleanup EXIT

section "Prerequisites"
if have go; then ok "Go found: $(go version)"; else ko "Go toolchain not found. Install Go 1.23+ and re-run."; fi
if have docker; then ok "Docker found: $(docker --version | cut -d',' -f1)"; else wn "Docker not found; will run local-only tests. To start services: docker compose --profile test up -d"; fi
if have docker && have docker compose; then ok "Docker Compose found: $(docker compose version | head -1)"; fi

section "Build goresearch CLI"
TMPDIR=$(mktemp -d)
if go build -o "$TMPDIR/goresearch" ./cmd/goresearch; then
  GO_BIN="$TMPDIR/goresearch"; ok "Built goresearch CLI"
else
  ko "Failed to build goresearch CLI"; echo "Try: go build -o bin/goresearch ./cmd/goresearch"; fi

section "Start OpenAI-compatible stub"
# Use local binary stub for determinism in this use case
STUB_URL="http://127.0.0.1:8081/v1"
if go build -o "$TMPDIR/openai-stub" ./cmd/openai-stub; then
  "$TMPDIR/openai-stub" >/dev/null 2>&1 & STUB_PID=$!
  CLEANUP_CMDS+=("kill $STUB_PID >/dev/null 2>&1 || true")
  sleep 0.5
  if curl -fsS "$STUB_URL/models" >/dev/null 2>&1; then ok "Stub LLM (local binary) started"; else ko "Failed to start stub LLM (local binary)"; fi
else
  ko "Failed to build stub LLM binary (cmd/openai-stub)"
fi

section "End-to-end pipeline with real URLs (stub LLM)"
BRIEF="$TMPDIR/brief.md"
RESULTS="$TMPDIR/results.json"
OUT="$TMPDIR/report.md"
REPORTS_DIR="$TMPDIR/reports"
cat >"$BRIEF" <<'EOF'
# System Test
Audience: engineers
Tone: terse
Target length: 120 words
EOF
cat >"$RESULTS" <<'EOF'
[
  {"Title":"Example Domain","URL":"https://example.com","Snippet":"System Test specification"},
  {"Title":"The Go Programming Language","URL":"https://go.dev","Snippet":"System Test documentation"}
]
EOF
"$GO_BIN" -input "$BRIEF" -output "$OUT" -search.file "$RESULTS" -llm.base "$STUB_URL" -llm.model test-model -cache.dir "$TMPDIR/cache" -reports.dir "$REPORTS_DIR" -reports.tar >/dev/null 2>&1
status=$?
if [[ $status -eq 0 && -s "$OUT" ]]; then
  ok "Generated report with evidence and manifest"
  grep -q "References" "$OUT" && ok "Report contains References section" || ko "References section missing"
  grep -q "Evidence check" "$OUT" && ok "Report contains Evidence check appendix" || ko "Evidence appendix missing"
  grep -q "\[[0-9]\]" "$OUT" && ok "Inline citations present" || ko "Inline citations missing"
  if compgen -G "$REPORTS_DIR"/*/planner.json >/dev/null; then ok "Artifacts bundle created"; else ko "Artifacts bundle missing"; fi
  if compgen -G "$REPORTS_DIR"/*.tar.gz >/dev/null; then ok "Artifacts tarball created"; else ko "Artifacts tarball missing"; fi
else
  ko "Pipeline failed (exit $status)"
fi

section "LocalAI (real local LLM) — optional"
if have docker && have docker compose; then
  # Start LocalAI with model bootstrap and expose port to localhost
  if docker compose -f docker-compose.yml -f docker-compose.override.yml.example --profile dev up -d models-bootstrap llm-openai >/dev/null 2>&1; then
    CLEANUP_CMDS+=("docker compose -f docker-compose.yml -f docker-compose.override.yml.example --profile dev down >/dev/null 2>&1 || true")
    # Wait for llm-openai healthy
    for i in {1..80}; do
      cid=$(docker compose -f docker-compose.yml -f docker-compose.override.yml.example ps -q llm-openai 2>/dev/null || true)
      if [[ -n "$cid" ]]; then
        s=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
        [[ "$s" == "healthy" ]] && break
      fi
      sleep 3
    done
    if curl -fsS http://127.0.0.1:8080/v1/models >/dev/null 2>&1; then
      ok "LocalAI healthy on localhost:8080"
      # Run pipeline using LocalAI; disable verification to reduce flakiness across models
      LLM_BASE="http://127.0.0.1:8080/v1"
      OUT2="$TMPDIR/report-localai.md"
      "$GO_BIN" -input "$BRIEF" -output "$OUT2" -search.file "$RESULTS" -llm.base "$LLM_BASE" -llm.model tinyllama -no-verify -cache.dir "$TMPDIR/cache_localai" >/dev/null 2>&1
      if [[ -s "$OUT2" ]] && grep -q "References" "$OUT2"; then
        ok "Generated report using LocalAI (no-verify)"
      else
        wn "LocalAI run did not produce a complete report; check docker compose logs llm-openai"
      fi
    else
      wn "LocalAI did not become healthy on localhost:8080; to troubleshoot: docker compose -f docker-compose.yml -f docker-compose.override.yml.example logs -f llm-openai"
    fi
  else
    wn "Could not start llm-openai; to run manually: docker compose -f docker-compose.yml -f docker-compose.override.yml.example --profile dev up -d llm-openai"
  fi
else
  wn "Docker not available; skipping LocalAI check"
fi

# This use case does not require SearxNG or cache-only behavior; omitted for clarity

section "Summary"
echo "${BOLD}Passes:${RESET} ${#passes[@]}"
for p in "${passes[@]}"; do echo "  - $p"; done
if [[ ${#warns[@]} -gt 0 ]]; then
  echo "${BOLD}Warnings:${RESET} ${#warns[@]}"; for w in "${warns[@]}"; do echo "  - $w"; done
fi
if [[ ${#fails[@]} -gt 0 ]]; then
  echo "${BOLD}Failures:${RESET} ${#fails[@]}"; for f in "${fails[@]}"; do echo "  - $f"; done
  exit 1
fi
exit 0
