# Build the server
build:
    @echo "Building the project..."
    @mkdir -p bin
    @go build -o bin/opus-mcp -ldflags "-X 'opus-mcp/internal/metadata.BuildVersion=v.0.0.1' -X 'opus-mcp/internal/metadata.BuildTime=$(date)'"

# Run the server with stdio transport
run-stdio: build
    @echo "Running the project..."
    @./bin/opus-mcp

# Run the server with http transport
run-http: build
    @echo "Running the project..."
    @./bin/opus-mcp -transport http

# Run tests
run-tests:
    @echo "Running tests..."
    @go test ./...

# Launch MCP Inspector for debugging
launch-inspector:
    #!/usr/bin/env bash
    echo "Launching MCP Inspector..."
    . ~/.nvm/nvm.sh && nvm use --lts && npx @modelcontextprotocol/inspector