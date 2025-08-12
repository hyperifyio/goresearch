.PHONY: wait up down logs rebuild test clean image image-archives builder-create test-build-amd64 test-build-arm64 test-multiarch build

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
	@echo "Running Go tests (local)"
	@go test ./...

# Build goresearch CLI locally (no docker). Output: bin/goresearch
build:
	@mkdir -p bin
	@echo "Building goresearch CLI -> bin/goresearch"
	@go build -o bin/goresearch ./cmd/goresearch

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
PLATFORMS ?= linux/amd64,linux/arm64

image:
	@echo "Building multi-arch goresearch image ($(PLATFORMS)) with SBOM and provenance"
	@docker buildx build \
		--platform=$(PLATFORMS) \
		--sbom=true --provenance=mode=max \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t ghcr.io/hyperifyio/goresearch:dev . || true

# Build and export local image archives for both architectures so developers on
# Intel and Apple Silicon can load and run without pushing to a registry.
image-archives:
	@mkdir -p dist
	@echo "Exporting per-arch image archives for linux/amd64 and linux/arm64"
	@docker buildx build \
		--platform=linux/amd64 \
		--sbom=true --provenance=mode=max \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t ghcr.io/hyperifyio/goresearch:dev \
		--output type=oci,dest=dist/goresearch_linux-amd64_$(VERSION).tar . || true
	@docker buildx build \
		--platform=linux/arm64 \
		--sbom=true --provenance=mode=max \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t ghcr.io/hyperifyio/goresearch:dev \
		--output type=oci,dest=dist/goresearch_linux-arm64_$(VERSION).tar . || true

# Convenience target to create a named builder with QEMU emulation locally
builder-create:
	@docker buildx create --use --name goresearch-builder --platform linux/amd64,linux/arm64 --driver docker-container >/dev/null 2>&1 || docker buildx use goresearch-builder
	@docker buildx inspect --bootstrap >/dev/null 2>&1

# Test build for linux/amd64 only (fast local testing)
test-build-amd64: builder-create
	@echo "Testing build for linux/amd64"
	@docker buildx build \
		--platform=linux/amd64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t goresearch:test-amd64 \
		--load .

# Test build for linux/arm64 only (with QEMU emulation)
test-build-arm64: builder-create
	@echo "Testing build for linux/arm64 (emulated)"
	@docker buildx build \
		--platform=linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t goresearch:test-arm64 .

# Test multi-arch build without pushing (validation)
test-multiarch: builder-create
	@echo "Testing multi-arch build for $(PLATFORMS)"
	@docker buildx build \
		--platform=$(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t goresearch:test-multiarch .
