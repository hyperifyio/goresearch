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

wait_until_ok() {
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

wait_until_ok "${_llm_models_url}" "llm-openai (/v1/models)"
wait_until_ok "${_searx_status_url}" "searxng (/status)"

echo "All dependencies healthy."
