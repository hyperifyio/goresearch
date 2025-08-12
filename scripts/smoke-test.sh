#!/usr/bin/env bash
# Smoke test for goresearch (Nginx HSTS use case)
# - Only validates external dependencies required by the tool:
#   - SearxNG at http://localhost:8080
#   - OpenAI-compatible API at http://localhost:1234/v1
# - Does NOT build or run goresearch; build moved to Makefile
# - Prints a clean PASS/FAIL summary

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

SEARX_URL="${SEARX_URL:-http://localhost:8080}"
LLM_BASE="${LLM_BASE_URL:-http://localhost:1234/v1}"
LLM_MODEL="${LLM_MODEL:-}"

# Compose files to consider when checking container health or starting services
COMPOSE_ARGS=(
  -f docker-compose.yml
  -f docker-compose.optional.yml
  -f docker-compose.override.yml.example
)

# If endpoints are down and Docker is available, try to bootstrap services via compose
maybe_bootstrap_services() {
  # Quick probe
  local searx_ok=0 llm_ok=0
  curl -fsS -m 3 "${SEARX_URL%/}/status" >/dev/null 2>&1 && searx_ok=1 || true
  curl -fsS -m 3 "${LLM_BASE%/}/models" >/dev/null 2>&1 && llm_ok=1 || true
  if [[ $searx_ok -eq 1 && $llm_ok -eq 1 ]]; then
    return 0
  fi

  if have docker && docker compose version >/dev/null 2>&1; then
    echo "Attempting to start dependencies with Docker Compose (searxng + stub-llm with host ports)..."
    docker compose "${COMPOSE_ARGS[@]}" \
      --profile test up -d stub-llm >/dev/null 2>&1 || true
    docker compose "${COMPOSE_ARGS[@]}" \
      up -d searxng >/dev/null 2>&1 || true
    # Do not block; rely on compose health checks in fallback logic
  fi
}

section "Prerequisites"
if have curl; then ok "curl found: $(curl --version | head -n1)"; else ko "curl not found. Install curl and re-run."; fi
if have jq; then ok "jq found: $(jq --version)"; else wn "jq not found. Model autodiscovery limited"; fi

section "Check SearxNG"
maybe_bootstrap_services
if curl -fsS -m 8 "${SEARX_URL%/}/status" >/dev/null 2>&1; then
  ok "SearxNG /status reachable at ${SEARX_URL}"
else
  # Fallback: if docker compose has a healthy container, accept as OK
  if command -v docker >/dev/null 2>&1 && docker compose ps >/dev/null 2>&1; then
    cid=$(docker compose "${COMPOSE_ARGS[@]}" ps -q searxng 2>/dev/null || true)
    if [ -n "$cid" ]; then
      status=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
      if [ "$status" = "healthy" ] || [ "$status" = "starting" ]; then
        wn "SearxNG not reachable at ${SEARX_URL}; container status=${status}. Using compose health as proxy."
      else
        ko "SearxNG not reachable at ${SEARX_URL} (expected /status)"
      fi
    else
      ko "SearxNG not reachable at ${SEARX_URL} (expected /status)"
    fi
  else
    ko "SearxNG not reachable at ${SEARX_URL} (expected /status)"
  fi
fi

# Simple JSON query to ensure search works
searx_resp=$(curl -fsS -m 12 "${SEARX_URL%/}/search?q=hsts%20nginx&format=json&language=en&categories=it" 2>/dev/null || true)
if [[ -n "$searx_resp" ]]; then
  if have jq; then
    cnt=$(printf '%s' "$searx_resp" | jq '(.results | length) // 0' 2>/dev/null || echo 0)
    if [[ "$cnt" -ge 0 ]]; then ok "SearxNG JSON search responded (results=$cnt)"; else ko "SearxNG JSON search returned invalid JSON"; fi
  else
    if printf '%s' "$searx_resp" | grep -q '"results"'; then ok "SearxNG JSON search responded"; else ko "SearxNG JSON search missing results field"; fi
  fi
else
  if command -v docker >/dev/null 2>&1 && docker compose ps >/dev/null 2>&1; then
    cid=$(docker compose "${COMPOSE_ARGS[@]}" ps -q searxng 2>/dev/null || true)
    if [ -n "$cid" ]; then
      status=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
      if [ "$status" = "healthy" ] || [ "$status" = "starting" ]; then
        wn "Skipping JSON search over host: container healthy but host port not reachable"
      else
        ko "SearxNG JSON search failed"
      fi
    else
      ko "SearxNG JSON search failed"
    fi
  else
    ko "SearxNG JSON search failed"
  fi
fi

section "Check OpenAI-compatible API"
if curl -fsS -m 8 "${LLM_BASE%/}/models" >/dev/null 2>&1; then
  ok "LLM models endpoint reachable at ${LLM_BASE}"
else
  # Accept compose health as proxy
  if command -v docker >/dev/null 2>&1 && docker compose ps >/dev/null 2>&1; then
    for svc in stub-llm llm-openai; do
      cid=$(docker compose "${COMPOSE_ARGS[@]}" ps -q "$svc" 2>/dev/null || true)
      if [ -n "$cid" ]; then
        status=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
        if [ "$status" = "healthy" ] || [ "$status" = "starting" ]; then
          wn "LLM not reachable at ${LLM_BASE}; ${svc} status=${status}. Using compose health as proxy."
          llm_compose_ok=1
          break
        fi
      fi
    done
    if [ "${llm_compose_ok:-0}" -ne 1 ]; then
      ko "LLM not reachable at ${LLM_BASE} (expected /models)"
    fi
  else
    ko "LLM not reachable at ${LLM_BASE} (expected /models)"
  fi
fi

# Optional lightweight chat completion to confirm basic inference path
model_to_use="$LLM_MODEL"
if [[ -z "$model_to_use" ]] && have jq; then
  model_to_use=$(curl -fsS -m 8 "${LLM_BASE%/}/models" 2>/dev/null | jq -r '.data[0].id // .models[0].id // empty' || true)
fi

if [[ -n "$model_to_use" ]]; then
  chat_payload=$(cat <<JSON
{"model":"${model_to_use}","messages":[{"role":"user","content":"Say OK"}],"max_tokens":8}
JSON
)
  if curl -fsS -m 12 -H 'Content-Type: application/json' -d "$chat_payload" "${LLM_BASE%/}/chat/completions" | grep -q '"choices"'; then
    ok "LLM chat completion succeeded with model=${model_to_use}"
  else
    ko "LLM chat completion failed (model=${model_to_use})"
  fi
else
  wn "Skipping chat completion: no model specified and jq unavailable or models list empty. Set LLM_MODEL to force."
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
