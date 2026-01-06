package metadata

const (
	APP_NAME  string = "opus-mcp"
	APP_TITLE string = "OPUS MCP Server"
)

var (
	BuildVersion string = "uninitialised; use linker flags: -X 'opus-mcp/internal/metadata.BuildVersion=1.0.0'"
	BuildTime    string = "uninitialised; use linker flags: -X 'opus-mcp/internal/metadata.BuildTime=$(date)'"
)
