# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Enable automatic toolchain download for dependencies requiring newer Go
ENV GOTOOLCHAIN=auto

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION
RUN set -eu; \
    build_version="$VERSION"; \
    if [ -z "$build_version" ]; then \
        release_file="$(git ls-files '.changes/v*.md' 2>/dev/null | sort -V | tail -n 1 || true)"; \
        if [ -n "$release_file" ]; then \
            release_name="${release_file##*/}"; \
            build_version="${release_name%.md}"; \
        else \
            build_version="0.1.0-dev"; \
        fi; \
    fi; \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-X github.com/GainForest/hyperindex/internal/buildinfo.Version=$build_version" -o /hyperindex ./cmd/hyperindex

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /hyperindex /app/hyperindex

# Copy static files (Quickslice client UI) if they exist
# Note: static directory may not exist yet during development
RUN mkdir -p /app/static

# Copy migrations (embedded in binary, but kept for reference)
# Note: migrations are embedded via go:embed, this line may be removed later

# Create data directory
RUN mkdir -p /app/data

# Expose port
EXPOSE 8080

# Set environment defaults
ENV HOST=0.0.0.0
ENV PORT=8080
ENV DATABASE_URL=sqlite:/app/data/hyperindex.db

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the server
ENTRYPOINT ["/app/hyperindex"]
