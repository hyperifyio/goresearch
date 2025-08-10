.PHONY: wait

# Wait for local dependencies (LLM and SearxNG) to become healthy.
# Uses environment variables LLM_BASE_URL and SEARX_URL when set.
wait:
	@bash scripts/wait-for-health.sh
