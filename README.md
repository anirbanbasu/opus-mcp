# Opus MCP

An MCP server for obtaining context information for language models on existing research.

## Quick Start

### Prerequisites

You'll need:
- Go 1.25.5 or later
- [`just`](https://github.com/casey/just?tab=readme-ov-file#installation) - command runner

### Running the Server

```bash
# Run with stdio transport (default)
just run-stdio

# Run with HTTP transport
just run-http
```

## Development Setup

### 1. Install Development Tools

```bash
just install-tools
```

This installs:
- **golangci-lint** - Comprehensive linter with 15+ enabled linters
- **gofumpt** - Stricter code formatter than gofmt
- **goimports** - Automatic import management
- **gosec** - Security vulnerability scanner
- **gitleaks** - Secret and credential detection

Add tools to your PATH (add to `~/.bashrc` or `~/.zshrc`):
```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

### 2. Install Pre-commit Hooks

Pre-commit hooks automatically check code quality before commits.

#### Option A: Using prek (Recommended - Faster)

[prek](https://github.com/asottile/prek) is a fast Rust-based alternative to pre-commit:

```bash
# Install prek
brew install asottile/prek/prek   # macOS/Linux
# or: cargo install prek

# Install hooks
just install-hooks
```

#### Option B: Manual Git Hook

```bash
just install-hooks-manual
```

### What the Hooks Check

The pre-commit hooks run:
- **Code formatting**: gofumpt and goimports
- **Linting**: golangci-lint (configured via `.golangci.yaml`)
- **Security**: gosec, gitleaks, detect-private-key, detect-aws-credentials
- **File quality**: Trailing whitespace, line endings (LF), JSON/YAML/TOML validation
- **Dependencies**: go mod tidy
- **Tests**: Full test suite

## Development Workflow

### Before Committing

Format and lint your code (hooks will run automatically, or run manually):

```bash
# Run all pre-commit checks
just pre-commit

# Format code
just fmt

# Run linter with auto-fix
just lint-fix
```

### Common Tasks

```bash
# Format code (gofumpt + goimports)
just fmt

# Run linter (report only)
just lint

# Run linter with auto-fix
just lint-fix

# Run security checks
just security

# Tidy dependencies
just tidy

# Run tests
just run-tests

# Build the project
just build
```

### Bypassing Pre-commit Checks

If absolutely necessary (not recommended):
```bash
git commit --no-verify -m "Your commit message"
```

## Configuration Files

- `.golangci.yaml` - golangci-lint configuration with 15+ linters
- `.pre-commit-config.yaml` - Pre-commit hooks configuration (compatible with prek and pre-commit)
- `.gitleaks.toml` - Secret detection configuration
- `justfile` - Task runner recipes

## Troubleshooting

**Tools not found:**
```bash
just install-tools
which golangci-lint gofumpt goimports
export PATH="$(go env GOPATH)/bin:$PATH"
```

**Pre-commit hook failing:**
```bash
# See detailed output
just pre-commit

# Fix most issues automatically
just fmt
just lint-fix
```
