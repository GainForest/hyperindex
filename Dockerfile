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
RUN CGO_ENABLED=0 GOOS=linux go build -o /hyperindex ./cmd/hyperindex

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
