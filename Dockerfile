# syntax=docker/dockerfile:1.7

# Build stage: compile goresearch as a static binary
# Pinned base image for reproducibility (update digest deliberately when upgrading)
FROM --platform=$BUILDPLATFORM golang:1.24-bookworm@sha256:6a0409c7c2dc6c9a31f41a13f5a3f6e1f2b0d8d44a4b8a3c7c5b4d2a8a7e1f0a AS build

ARG VERSION=0.0.0
ARG COMMIT=dev
ARG DATE=1970-01-01T00:00:00Z

WORKDIR /src

# Enable Go modules and caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of the source
COPY . .

# Build the CLI (multi-arch via TARGETOS/TARGETARCH)
# TARGETOS/TARGETARCH are provided by BuildKit automatically
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X 'github.com/hyperifyio/goresearch/internal/app.BuildVersion=${VERSION}' -X 'github.com/hyperifyio/goresearch/internal/app.BuildCommit=${COMMIT}' -X 'github.com/hyperifyio/goresearch/internal/app.BuildDate=${DATE}'" \
    -o /out/goresearch ./cmd/goresearch

# Runtime stage: non-root, minimal image with certs
FROM gcr.io/distroless/static-debian12:nonroot@sha256:f4a3d58ee4f1f0b1a6a3be5e6f2b7e6a5f0c9d4b3a2f1e0d9c8b7a6f5e4d3c2b

# OCI labels for provenance
ARG VERSION=0.0.0
ARG COMMIT=dev
ARG DATE=1970-01-01T00:00:00Z
LABEL org.opencontainers.image.title="goresearch" \
      org.opencontainers.image.description="Generate validated, citation-rich research reports from a single Markdown brief." \
      org.opencontainers.image.url="https://github.com/hyperifyio/goresearch" \
      org.opencontainers.image.source="https://github.com/hyperifyio/goresearch" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$DATE"

WORKDIR /app

# Copy binary
COPY --from=build /out/goresearch /usr/local/bin/goresearch

# Provide a tiny healthcheck input inside the image
# Keep content minimal to exercise dry-run path without network calls.
COPY <<'EOF' /app/healthcheck.md
# Healthcheck Topic
Audience: engineers
Tone: terse
Target length: 10 words

Key questions: hello world
EOF

# Writable volumes for reports and cache
VOLUME ["/app/reports", "/app/.goresearch-cache"]

# Default non-secret environment can be overridden at runtime. Do not bake
# secrets like API keys into the image.
ENV LLM_BASE_URL="http://llm-openai:8080/v1" \
    LLM_MODEL="gpt-neo" \
    SEARX_URL="http://searxng:8080"

# Healthcheck: quick dry-run that must exit 0 on success
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/usr/local/bin/goresearch", "-dry-run", "-input", "/app/healthcheck.md", "-output", "/tmp/health-report.md", "-searx.url", "${SEARX_URL}"]

# Non-root entrypoint
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/goresearch"]
# No default CMD; supply flags/env at runtime
