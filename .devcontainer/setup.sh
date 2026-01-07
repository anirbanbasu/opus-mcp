#!/bin/bash
set -e

echo "🚀 Setting up Opus MCP development environment..."

# Copy .gitconfig from host if it exists and hasn't been copied yet
if [ -f "/tmp/.host-gitconfig-cache/gitconfig" ] && [ ! -f "/home/vscode/.gitconfig" ]; then
    echo "📋 Copying Git configuration from host..."
    cp /tmp/.host-gitconfig-cache/gitconfig /home/vscode/.gitconfig
    chmod 644 /home/vscode/.gitconfig
fi

# Install Homebrew
echo "🍺 Installing Homebrew..."
NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
echo >> /home/vscode/.bashrc
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> /home/vscode/.bashrc
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"

# Install just (task runner)
echo "📦 Installing just..."
brew install just

# Install Go tools
echo "🔧 Installing Go development tools..."

# Install Go
echo "  → Go"
brew install go

echo >> /home/vscode/.bashrc
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> /home/vscode/.bashrc
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

# Install pre-commit hooks
echo "🔩 Installing pre-commit hooks..."
just install-hooks

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
command -v goimports >/dev/null && echo "goimports installed" || echo "goimports not found"
gosec --version
gitleaks version
prek --version
echo ""
echo "🎉 Run 'just run-http' or 'just run-stdio' to start the server"
