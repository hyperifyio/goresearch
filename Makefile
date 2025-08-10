.PHONY: wait up down logs rebuild test clean image

# Wait for local dependencies (LLM and SearxNG) to become healthy.
# Uses environment variables LLM_BASE_URL and SEARX_URL when set.
wait:
	@bash scripts/wait-for-health.sh

# Bring up the local dev stack (profiles: dev)
up:
	@echo "Starting dev stack (profiles: dev)"
	@docker compose --profile dev up -d

# Tear down the stack without removing named volumes
down:
	@echo "Stopping stack (keeping volumes)"
	@docker compose down

# Follow logs for all services
logs:
	@docker compose logs -f --tail=200

# Rebuild and recreate dev services
rebuild:
	@echo "Rebuilding and recreating dev services"
	@docker compose --profile dev up -d --build --force-recreate

# Run Go tests; optionally bring up the test profile with stub-llm
# Note: In environments without Docker, this still runs local tests.
test:
	@echo "Running Go tests (local). If Docker is available, starting test profile with stub-llm..."
	@docker compose --profile test up -d stub-llm >/dev/null 2>&1 || true
	@go test ./...
	@docker compose --profile test down >/dev/null 2>&1 || true

# Prune only cache-related volumes created by this project
# Safe to run even if Docker is unavailable
clean:
	@echo "Pruning cache volumes and local cache directory"
	@docker volume rm goresearch_http_cache goresearch_llm_cache >/dev/null 2>&1 || true
	@rm -rf .goresearch-cache || true

# Build the goresearch container image with SBOM and provenance attestations.
# Note: This target only constructs the image; it does not push anywhere.
# It relies on Docker Buildx and BuildKit; in environments without Docker,
# this target is a no-op unless Docker is available.
VERSION ?= 0.0.0-dev
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo dev)
DATE    ?= $(shell date -u +%FT%TZ)

image:
	@echo "Building goresearch image with SBOM and provenance"
	@docker buildx build \
		--sbom=true --provenance=mode=max \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t ghcr.io/hyperifyio/goresearch:dev . || true
