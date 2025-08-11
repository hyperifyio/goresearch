#!/usr/bin/env bash
set -euo pipefail

# Traceability: Implements FEATURE_CHECKLIST.md item
# "Health-gated startup — ... Provide a make wait target that polls health for local troubleshooting."

LLM_BASE_URL="${LLM_BASE_URL:-http://llm-openai:8080/v1}"
SEARX_URL="${SEARX_URL:-http://searxng:8080}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-120}"
SLEEP_SECONDS="${SLEEP_SECONDS:-3}"

# Normalize URLs and compute endpoints
_llm_models_url="${LLM_BASE_URL%/}/models"
_searx_status_url="${SEARX_URL%/}/status"

echo "Waiting for dependencies to become healthy..."
echo "  LLM models endpoint: ${_llm_models_url}"
echo "  SearxNG status:      ${_searx_status_url}"

deadline=$(( $(date +%s) + TIMEOUT_SECONDS ))

wait_until_ok_http() {
  local url="$1"
  local name="$2"
  while true; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "${name} healthy"
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "ERROR: Timed out after ${TIMEOUT_SECONDS}s waiting for ${name} at ${url}" >&2
      return 1
    fi
    echo "Waiting for ${name}... retrying in ${SLEEP_SECONDS}s"
    sleep "${SLEEP_SECONDS}"
  done
}

wait_until_healthy_compose() {
  local svc="$1"; shift
  local name="$1"; shift
  while true; do
    cid=$(docker compose ps -q "$svc" 2>/dev/null || true)
    if [ -n "$cid" ]; then
      status=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
      if [ "$status" = "healthy" ]; then
        echo "$name healthy (compose)"
        return 0
      fi
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "ERROR: Timed out after ${TIMEOUT_SECONDS}s waiting for ${name} (compose health)" >&2
      return 1
    fi
    echo "Waiting for ${name} (compose)... retrying in ${SLEEP_SECONDS}s"
    sleep "${SLEEP_SECONDS}"
  done
}

# If docker compose is available and services are part of a compose project,
# prefer container health checks (works with internal networks).
if command -v docker >/dev/null 2>&1 && docker compose ps >/dev/null 2>&1; then
  wait_until_healthy_compose llm-openai "llm-openai (/v1/models)" || exit 1
  # searxng has a healthcheck in compose, so rely on compose status
  wait_until_healthy_compose searxng "searxng (/status)" || exit 1
else
  # Fallback to host HTTP polling
  wait_until_ok_http "${_llm_models_url}" "llm-openai (/v1/models)" || exit 1
  wait_until_ok_http "${_searx_status_url}" "searxng (/status)" || exit 1
fi

echo "All dependencies healthy."
