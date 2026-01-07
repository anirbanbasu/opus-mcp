package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"opus-mcp/internal/parser"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const arxivApiEndpoint string = "http://export.arxiv.org/api/query"

func mcp_tool_errorf(format string, args ...any) *mcp.CallToolResult {
	slog.Warn(fmt.Sprintf(format, args...))
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}
}

func unmarshalAndValidate(data []byte, res *jsonschema.Resolved) error {
	/*
		Unmarshal the given data into a map[string]any and validate it against the given JSON schema.
	*/
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	return res.Validate(m)
}

type ArxivCategoryFetchLatestArgs struct {
	Category             string `json:"category" jsonschema:"The arXiv categories to fetch latest publications from. See taxonomy at https://arxiv.org/category_taxonomy"`
	CategoryJoinStrategy string `json:"categoryJoinStrategy,omitempty" jsonschema:"Strategy to join multiple categories. Valid values are 'AND' or 'OR'. Defaults to 'AND' if not provided. This has no effect if only one category is provided"`
	StartIndex           uint   `json:"startIndex,omitempty" jsonschema:"The starting index of results to fetch (0-based)"`
	FetchSize            uint   `json:"fetchSize,omitempty" jsonschema:"The number of results to fetch"`
}

type CategoryFetchLatestOutput struct {
	Results string `json:"results" jsonschema:"The latest publications from arXiv in JSON format"`
}

type ArxivWrapper struct {
	inputSchema  *jsonschema.Resolved
	outputSchema *jsonschema.Resolved
}

func NewArxivWrapper(inputSchema, outputSchema *jsonschema.Schema) (*ArxivWrapper, error) {
	resIn, err := inputSchema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	resOut, err := outputSchema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	return &ArxivWrapper{
		inputSchema:  resIn,
		outputSchema: resOut,
	}, nil
}

func (t *ArxivWrapper) CategoryFetchLatest(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// First, unmarshal to a map[string]any and validate.
	if err := unmarshalAndValidate(req.Params.Arguments, t.inputSchema); err != nil {
		return mcp_tool_errorf("invalid input: %v", err), nil
	}
	// Now unmarshal again to input.
	var input ArxivCategoryFetchLatestArgs
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return mcp_tool_errorf("failed to unmarshal arguments: %v", err), nil
	}

	// Build search query for multiple categories
	searchQuery, err := parser.ParseReconstructCategoryExpression(input.Category)
	if err != nil {
		return mcp_tool_errorf("failed to parse category expression: %v", err), nil
	}

	// Fetch contents from arXiv API
	url := arxivApiEndpoint + "?search_query=" + searchQuery + "&start=" + fmt.Sprint(input.StartIndex) + "&max_results=" + fmt.Sprint(input.FetchSize) + "&sortBy=submittedDate&sortOrder=descending"
	slog.Info("Fetching Atom feed from arXiv", "url", url)
	// #nosec G107 -- URL is constructed from constant arxivApiEndpoint, query params are safe
	resp, err := http.Get(url)
	if err != nil {
		return mcp_tool_errorf("failed to fetch from arXiv: %v", err), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp_tool_errorf("failed to read response body: %v", err), nil
	}

	fp := gofeed.NewParser()
	output, err := fp.ParseString(string(body))
	if err != nil {
		return mcp_tool_errorf("failed to parse feed: %v", err), nil
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return mcp_tool_errorf("output failed to marshal: %v", err), nil
	}
	//
	if err := unmarshalAndValidate(outputJSON, t.outputSchema); err != nil {
		return mcp_tool_errorf("invalid output: %v", err), nil
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(outputJSON)}},
		StructuredContent: output,
	}, nil
}
