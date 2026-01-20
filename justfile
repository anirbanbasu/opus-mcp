# Build the server
build:
    @echo "Building the project..."
    @mkdir -p bin
    @go build -o bin/opus-mcp -ldflags "-X 'opus-mcp/internal/metadata.BuildVersion=v.0.0.1' -X 'opus-mcp/internal/metadata.BuildTime=$(date)'"

# Install development tools (golangci-lint, gofumpt, goimports, gosec)
install-tools:
    @echo "Installing development tools..."
    @echo "→ Installing golangci-lint..."
    @if command -v brew >/dev/null 2>&1; then \
        brew install golangci-lint; \
    else \
        go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
    fi
    @echo "→ Installing gofumpt..."
    @go install mvdan.cc/gofumpt@latest
    @echo "→ Installing goimports..."
    @go install golang.org/x/tools/cmd/goimports@latest
    @echo "→ Installing gosec (security checker)..."
    @go install github.com/securego/gosec/v2/cmd/gosec@latest
    @echo "✓ All tools installed!"
    @echo "  Make sure $(go env GOPATH)/bin is in your PATH"

# Run the server with stdio transport
run-stdio: build
    @echo "Running the project..."
    @./bin/opus-mcp

# Run the server with http transport
run-http: build
    @echo "Running the project..."
    @./bin/opus-mcp -transport http

# Run tests with linker flags for metadata package
test-metadata-with-flags:
    @echo "Testing metadata package with linker flags..."
    @go test -v ./internal/metadata -ldflags "-X 'opus-mcp/internal/metadata.BuildVersion=v1.2.3-test' -X 'opus-mcp/internal/metadata.BuildTime=$(date -Iseconds)'"

# Run tests
run-tests: test-metadata-with-flags
    @echo "Running all tests..."
    @go test -v ./...

# Launch MCP Inspector for debugging
launch-inspector:
    #!/usr/bin/env bash
    echo "Launching MCP Inspector..."
    . ~/.nvm/nvm.sh && nvm use --lts && npx @modelcontextprotocol/inspector

# Format Go code with gofumpt (stricter than gofmt)
fmt:
    @echo "Formatting Go code with gofumpt..."
    @gofumpt -w .
    @goimports -w .
    @echo "✓ Code formatted"

# Run golangci-lint
lint:
    @echo "Running golangci-lint..."
    @golangci-lint run
    @echo "✓ Linting passed"

# Run golangci-lint with auto-fix
lint-fix:
    @echo "Running golangci-lint with auto-fix..."
    @golangci-lint run --fix
    @echo "✓ Linting passed (with fixes applied)"

# Run gosec security checker
security:
    @echo "Running gosec security checks..."
    @gosec ./...
    @echo "✓ Security checks passed"

# Tidy go modules
tidy:
    @echo "Running go mod tidy..."
    @go mod tidy
    @echo "✓ Dependencies tidied"

# Run all pre-commit checks
pre-commit: fmt lint-fix tidy run-tests security
    @echo "✓ All pre-commit checks passed!"

# Update Go module dependencies
update-deps:
    @echo "Updating Go module dependencies..."
    @go get -u ./...
    @go mod tidy
    @echo "✓ Dependencies updated"

# Install pre-commit hooks (using prek - faster than pre-commit)
install-hooks:
    @echo "Installing pre-commit hooks with prek..."
    @if command -v prek >/dev/null 2>&1; then \
        prek install; \
        echo "✓ Pre-commit hooks installed via prek"; \
    elif command -v pre-commit >/dev/null 2>&1; then \
        pre-commit install; \
        echo "✓ Pre-commit hooks installed via pre-commit"; \
    else \
        echo "⚠ Neither prek nor pre-commit found."; \
        echo "  Install prek with: brew install asottile/prek/prek (recommended)"; \
        echo "  Or install pre-commit with: pip install pre-commit"; \
        echo "  Or use manual hooks: just install-hooks-manual"; \
    fi

# Install manual git hooks (without pre-commit framework)
install-hooks-manual:
    @echo "Installing manual git hooks..."
    @cp scripts/pre-commit.sh .git/hooks/pre-commit
    @chmod +x .git/hooks/pre-commit
    @echo "✓ Manual pre-commit hook installed"
