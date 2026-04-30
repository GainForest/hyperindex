.PHONY: help build run test test-coverage lint fmt clean dev db-migrate db-rollback db-status db-create-migration docker docker-run tools generate hooks-install changie-new

GO_TOOLCHAIN := GOTOOLCHAIN=go1.26.0
VERSION ?= 0.1.0-dev
LDFLAGS := -X github.com/GainForest/hyperindex/internal/buildinfo.Version=$(VERSION)

# Default target
help:
	@echo "Hyperindex - Makefile Commands"
	@echo ""
	@echo "Development:"
	@echo "  make run          - Run the server"
	@echo "  make dev          - Run with hot reload (requires air)"
	@echo "  make build        - Build the binary"
	@echo "  make test         - Run all tests"
	@echo "  make lint         - Run linter"
	@echo "  make tools        - Install development tools (including Changie)"
	@echo "  make changie-new  - Create a new changelog fragment"
	@echo "  make hooks-install - Install local git hooks path"
	@echo "  make clean        - Clean build artifacts"
	@echo ""
	@echo "Database:"
	@echo "  make db-migrate   - Run database migrations"
	@echo "  make db-rollback  - Rollback last migration"
	@echo "  make db-status    - Show migration status"
	@echo ""
	@echo "Docker:"
	@echo "  make docker       - Build Docker image"
	@echo "  make docker-run   - Run with Docker Compose"
	@echo ""

# Build the binary
build:
	@echo "Building hyperindex..."
	@go build -ldflags "$(LDFLAGS)" -o bin/hyperindex ./cmd/hyperindex

# Run the server
run: build
	@echo "Starting hyperindex server..."
	@./bin/hyperindex

# Run with hot reload (requires air: go install github.com/air-verse/air@latest)
dev:
	@air

# Run all tests
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@gofumpt -l -w .

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf dist/
	@rm -f coverage.out coverage.html
	@go clean

# Database migrations (requires golang-migrate)
db-migrate:
	@echo "Running migrations..."
	@migrate -path db/migrations -database "$${DATABASE_URL}" up

db-rollback:
	@echo "Rolling back last migration..."
	@migrate -path db/migrations -database "$${DATABASE_URL}" down 1

db-status:
	@echo "Migration status..."
	@migrate -path db/migrations -database "$${DATABASE_URL}" version

db-create-migration:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir db/migrations -seq $$name

# Docker
docker:
	@echo "Building Docker image..."
	@docker build -t hyperindex:latest .

docker-run:
	@echo "Starting with Docker Compose..."
	@docker compose up --build

# Install development tools
tools:
	@echo "Installing development tools..."
	@$(GO_TOOLCHAIN) go install github.com/air-verse/air@latest
	@$(GO_TOOLCHAIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@$(GO_TOOLCHAIN) go install mvdan.cc/gofumpt@latest
	@$(GO_TOOLCHAIN) go install -tags 'postgres sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@$(GO_TOOLCHAIN) go install github.com/miniscruff/changie@v1.24.0

# Generate (placeholder for future code generation)
generate:
	@echo "Running go generate..."
	@go generate ./...

# Create a new changelog fragment with Changie
changie-new:
	@changie new

# Install local git hooks path for tracked hooks
hooks-install:
	@echo "Configuring git to use .githooks..."
	@git config core.hooksPath .githooks
	@echo "Installed. Hooks path: $$(git config --get core.hooksPath)"
