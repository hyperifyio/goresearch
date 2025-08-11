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
# Prefer Docker stub if Docker is available, else run local stub binary
STUB_URL="http://127.0.0.1:8081/v1"
if have docker && have docker compose; then
  # Build and run the in-repo stub container
  if docker compose --profile test build stub-llm >/dev/null; then
    if docker compose --profile test up -d stub-llm >/dev/null; then
      CLEANUP_CMDS+=("docker compose --profile test down >/dev/null 2>&1 || true")
      # Wait for health
      for i in {1..60}; do
        cid=$(docker compose ps -q stub-llm 2>/dev/null || true)
        if [[ -n "$cid" ]]; then
          s=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
          [[ "$s" == "healthy" ]] && break
        fi
        sleep 2
      done
      if curl -fsS "$STUB_URL/models" >/dev/null 2>&1; then ok "Stub LLM (container) healthy"; else ko "Stub LLM (container) did not become healthy"; fi
    else
      wn "Could not start stub-llm container; falling back to local binary stub"
    fi
  else
    wn "Could not build stub-llm container; falling back to local binary stub"
  fi
fi

if ! curl -fsS "$STUB_URL/models" >/dev/null 2>&1; then
  # Run local stub server binary
  if go build -o "$TMPDIR/openai-stub" ./cmd/openai-stub; then
    "$TMPDIR/openai-stub" >/dev/null 2>&1 & STUB_PID=$!
    CLEANUP_CMDS+=("kill $STUB_PID >/dev/null 2>&1 || true")
    # Wait a moment
    sleep 0.5
    if curl -fsS "$STUB_URL/models" >/dev/null 2>&1; then ok "Stub LLM (local binary) started"; else ko "Failed to start stub LLM (local binary)"; fi
  else
    ko "Failed to build stub LLM binary (cmd/openai-stub)"
  fi
fi

section "End-to-end pipeline with real URLs (stub LLM)"
BRIEF="$TMPDIR/brief.md"
RESULTS="$TMPDIR/results.json"
OUT="$TMPDIR/report.md"
REPORTS_DIR="$TMPDIR/reports"
cat >"$BRIEF" <<'EOF'
# Smoke Test: Real Run
Audience: engineers
Tone: terse
Target length: 120 words
EOF
cat >"$RESULTS" <<'EOF'
[
  {"Title":"Example Domain","URL":"https://example.com","Snippet":"Smoke Test specification"},
  {"Title":"The Go Programming Language","URL":"https://go.dev","Snippet":"Smoke Test documentation"}
]
EOF
"$GO_BIN" -input "$BRIEF" -output "$OUT" -search.file "$RESULTS" -llm.base "$STUB_URL" -llm.model test-model -cache.dir "$TMPDIR/cache" -reports.dir "$REPORTS_DIR" -reports.tar >/dev/null 2>&1
status=$?
if [[ $status -eq 0 && -s "$OUT" ]]; then
  ok "Generated report with evidence and manifest"
  grep -q "References" "$OUT" && ok "Report contains References section" || ko "References section missing"
  grep -q "Evidence check" "$OUT" && ok "Report contains Evidence check appendix" || ko "Evidence appendix missing"
  grep -q "\[[0-9]\]" "$OUT" && ok "Inline citations present" || ko "Inline citations missing"
  test -f "$REPORTS_DIR/smoke-test-real-run/planner.json" && ok "Artifacts bundle created" || ko "Artifacts bundle missing"
  test -f "$REPORTS_DIR/smoke-test-real-run.tar.gz" && ok "Artifacts tarball created" || ko "Artifacts tarball missing"
else
  ko "Pipeline failed (exit $status)"
fi

section "Offline cache-only fail-fast"
# Expect failure when HTTPCacheOnly=1 and no cache entry exists
"$GO_BIN" -input "$BRIEF" -output "$TMPDIR/out2.md" -search.file "$RESULTS" -cache.dir "$TMPDIR/cache2" -cache.strictPerms -cache.topicHash deadbeef -dry-run -llm.model "" >/dev/null 2>&1 || true
# Above is a dry-run; now enforce HTTP cache only with a selected URL that won't exist in cache
"$GO_BIN" -input "$BRIEF" -output "$TMPDIR/out3.md" -search.file "$RESULTS" -cache.dir "$TMPDIR/cache3" -llm.base "$STUB_URL" -llm.model test-model -robots.overrideConfirm=false >/dev/null 2>&1 || true
# Clear and enforce cache-only to provoke a miss
rm -rf "$TMPDIR/cache3" && mkdir -p "$TMPDIR/cache3"
if "$GO_BIN" -input "$BRIEF" -output "$TMPDIR/out4.md" -search.file "$RESULTS" -cache.dir "$TMPDIR/cache3" -llm.base "$STUB_URL" -llm.model test-model -http_cache_only >/dev/null 2>&1; then
  ko "Expected cache-only mode to fail on cache miss, but it succeeded"
else
  ok "Cache-only mode fails fast on HTTP cache miss"
fi

section "Docker Compose services (optional)"
if have docker && have docker compose; then
  # SearxNG health (internal-only)
  if docker compose --profile dev up -d searxng >/dev/null; then
    CLEANUP_CMDS+=("docker compose --profile dev down >/dev/null 2>&1 || true")
    for i in {1..40}; do
      cid=$(docker compose ps -q searxng 2>/dev/null || true)
      [[ -n "$cid" ]] || { sleep 2; continue; }
      s=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
      [[ "$s" == healthy ]] && break
      sleep 3
    done
    cid=$(docker compose ps -q searxng 2>/dev/null || true)
    if [[ -n "$cid" ]]; then
      s=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
      [[ "$s" == healthy ]] && ok "SearxNG container became healthy" || wn "SearxNG did not become healthy yet; check 'docker compose logs searxng'"
    else
      wn "SearxNG container not found"
    fi
  else
    wn "Could not start searxng; to run manually: docker compose --profile dev up -d searxng"
  fi
else
  wn "Docker not available; to run services: docker compose --profile test up -d stub-llm"
fi

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
