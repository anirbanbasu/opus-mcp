#!/usr/bin/env bash
# Git pre-commit hook without prek/pre-commit framework
# Copy this file to .git/hooks/pre-commit and make it executable:
#   cp scripts/pre-commit.sh .git/hooks/pre-commit
#   chmod +x .git/hooks/pre-commit

set -e

echo "Running pre-commit checks..."

# Check if required tools are installed
for tool in gofumpt goimports golangci-lint; do
    if ! command -v $tool &> /dev/null; then
        echo "⚠ $tool is not installed. Please run:"
        echo "  go install mvdan.cc/gofumpt@latest  # for gofumpt"
        echo "  go install golang.org/x/tools/cmd/goimports@latest  # for goimports"
        echo "  brew install golangci-lint  # for golangci-lint"
        exit 1
    fi
done

# Format Go code with gofumpt
echo "→ Running gofumpt..."
if gofumpt -l . | grep -v vendor | grep -q .; then
    echo "✗ Code formatting issues found. Running gofumpt -w..."
    gofumpt -w .
    goimports -w .
    echo "  Code has been formatted. Please review and commit again."
    exit 1
else
    echo "✓ Code is formatted"
fi

# Run goimports
echo "→ Running goimports..."
goimports -w .
if git diff --exit-code -- '*.go' > /dev/null 2>&1; then
    echo "✓ Imports are organized"
else
    echo "✗ Imports were reorganized. Please review and commit again."
    exit 1
fi

# Run golangci-lint
echo "→ Running golangci-lint..."
if golangci-lint run --fix; then
    echo "✓ golangci-lint passed"
else
    echo "✗ golangci-lint found issues"
    exit 1
fi

# Check if golangci-lint made any changes
if ! git diff --exit-code -- '*.go' > /dev/null 2>&1; then
    echo "✗ golangci-lint made fixes. Please review and commit again."
    exit 1
fi

# Run go mod tidy
echo "→ Running go mod tidy..."
go mod tidy
if git diff --exit-code go.mod go.sum > /dev/null 2>&1; then
    echo "✓ go.mod and go.sum are tidy"
else
    echo "✗ go.mod or go.sum changed. Please review and commit:"
    git diff go.mod go.sum
    exit 1
fi

# Run tests
echo "→ Running tests..."
if just run-tests > /dev/null 2>&1; then
    echo "✓ All tests passed"
else
    echo "✗ Tests failed"
    just run-tests
    exit 1
fi

echo "✓ All pre-commit checks passed!"
