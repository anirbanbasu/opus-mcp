#!/bin/bash
set -e

echo "🚀 Setting up Opus MCP development environment..."

# Install Homebrew
echo "🍺 Installing Homebrew..."
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
echo >> /home/vscode/.bashrc
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> /home/vscode/.bashrc
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"

# Install just (task runner)
echo "📦 Installing just..."
brew install just

# Install Go tools
echo "🔧 Installing Go development tools..."
export PATH="$(go env GOPATH)/bin:$PATH"

# Install golangci-lint
echo "  → golangci-lint"
brew install golangci-lint

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
brew install gitleaks

# Install prek
echo "  → prek"
brew install prek

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
