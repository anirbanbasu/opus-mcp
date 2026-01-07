#!/bin/bash
set -e

echo "🚀 Setting up Opus MCP development environment..."

# Install just (task runner)
echo "📦 Installing just..."
curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash -s -- --to /usr/local/bin

# Install Go tools
echo "🔧 Installing Go development tools..."
export PATH="$(go env GOPATH)/bin:$PATH"

# Install golangci-lint
echo "  → golangci-lint"
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.62.2

# Install gofumpt
echo "  → gofumpt"
go install mvdan.cc/gofumpt@latest

# Install goimports
echo "  → goimports"
go install golang.org/x/tools/cmd/goimports@latest

# Install gosec
echo "  → gosec"
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Install gitleaks
echo "  → gitleaks"
GITLEAKS_VERSION="8.24.2"
curl -sSfL "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz" | tar -xz -C /usr/local/bin gitleaks
chmod +x /usr/local/bin/gitleaks

# Install prek
echo "  → prek"
curl --proto '=https' --tlsv1.2 -LsSf https://github.com/j178/prek/releases/download/v0.2.25/prek-installer.sh | sh

# Download Go dependencies
echo "📚 Downloading Go dependencies..."
go mod download

# Verify installations
echo ""
echo "✅ Development environment ready!"
echo ""
echo "Installed tools:"
just --version
go version
golangci-lint --version
gofumpt -version
goimports -version 2>&1 | head -1 || echo "goimports installed"
gosec --version
gitleaks version
prek --version
echo ""
echo "🎉 Run 'just run-http' or 'just run-stdio' to start the server"
