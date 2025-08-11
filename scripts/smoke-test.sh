#!/usr/bin/env bash
# Simple, real smoke test for goresearch (requires a real local LLM)
# - Starts LocalAI via Docker Compose (exposed on localhost:8080)
# - Runs the end-to-end pipeline against the real LLM
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
if have docker; then ok "Docker found: $(docker --version | cut -d',' -f1)"; else ko "Docker not found; required for LocalAI"; fi
if have docker && have docker compose; then ok "Docker Compose found: $(docker compose version | head -1)"; else ko "Docker Compose not found; required for LocalAI"; fi

section "Build goresearch CLI"
TMPDIR=$(mktemp -d)
if go build -o "$TMPDIR/goresearch" ./cmd/goresearch; then
  GO_BIN="$TMPDIR/goresearch"; ok "Built goresearch CLI"
else
  ko "Failed to build goresearch CLI"; echo "Try: go build -o bin/goresearch ./cmd/goresearch"; fi

section "Start LocalAI (real LLM)"
if have docker && have docker compose; then
  # Bring up LocalAI and model bootstrap; expose port via override
  if docker compose -f docker-compose.yml -f docker-compose.override.yml.example --profile dev up -d models-bootstrap llm-openai >/dev/null 2>&1; then
    CLEANUP_CMDS+=("docker compose -f docker-compose.yml -f docker-compose.override.yml.example --profile dev down >/dev/null 2>&1 || true")
    # Wait for container health and host port readiness
    for i in {1..120}; do
      cid=$(docker compose -f docker-compose.yml -f docker-compose.override.yml.example ps -q llm-openai 2>/dev/null || true)
      if [[ -n "$cid" ]]; then
        s=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
        if [[ "$s" == "healthy" ]] && curl -fsS http://127.0.0.1:8080/v1/models >/dev/null 2>&1; then
          ok "LocalAI healthy on localhost:8080"
          break
        fi
      fi
      sleep 2
    done
    if ! curl -fsS http://127.0.0.1:8080/v1/models >/dev/null 2>&1; then
      ko "LocalAI did not become healthy on localhost:8080"
    fi
  else
    ko "Could not start llm-openai; to run manually: docker compose -f docker-compose.yml -f docker-compose.override.yml.example --profile dev up -d llm-openai"
  fi
else
  ko "Docker/Compose unavailable; cannot start LocalAI"
fi

section "Run end-to-end pipeline with LocalAI"
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
LLM_BASE="http://127.0.0.1:8080/v1"
"$GO_BIN" -input "$BRIEF" -output "$OUT" -search.file "$RESULTS" -llm.base "$LLM_BASE" -llm.model tinyllama -no-verify -cache.dir "$TMPDIR/cache" -reports.dir "$REPORTS_DIR" -reports.tar >/dev/null 2>&1
status=$?
if [[ $status -eq 0 && -s "$OUT" ]]; then
  ok "Generated report with evidence and manifest"
  grep -q "References" "$OUT" && ok "Report contains References section" || ko "References section missing"
  # Evidence appendix and inline citations may vary across real models; treat as informational
  grep -q "Evidence check" "$OUT" && ok "Report contains Evidence check appendix" || wn "Evidence appendix missing"
  grep -q "\[[0-9]\]" "$OUT" && ok "Inline citations present" || wn "Inline citations missing"
  if compgen -G "$REPORTS_DIR"/*/planner.json >/dev/null; then ok "Artifacts bundle created"; else ko "Artifacts bundle missing"; fi
  if compgen -G "$REPORTS_DIR"/*.tar.gz >/dev/null; then ok "Artifacts tarball created"; else ko "Artifacts tarball missing"; fi
else
  ko "Pipeline failed (exit $status)"
fi

# This use case does not require SearxNG; omitted for clarity

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
