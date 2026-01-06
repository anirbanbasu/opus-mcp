package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const arxivApiEndpoint string = "http://export.arxiv.org/api/query"

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
	Category             []string `json:"category" jsonschema:"The arXiv categories to fetch latest publications from. See taxonomy at https://arxiv.org/category_taxonomy"`
	CategoryJoinStrategy string   `json:"categoryJoinStrategy,omitempty" jsonschema:"Strategy to join multiple categories. Valid values are 'AND' or 'OR'. Defaults to 'AND' if not provided. This has no effect if only one category is provided"`
	StartIndex           uint     `json:"startIndex,omitempty" jsonschema:"The starting index of results to fetch (0-based)"`
	FetchSize            uint     `json:"fetchSize,omitempty" jsonschema:"The number of results to fetch"`
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
	errf := func(format string, args ...any) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
			IsError: true,
		}
	}
	// First, unmarshal to a map[string]any and validate.
	if err := unmarshalAndValidate(req.Params.Arguments, t.inputSchema); err != nil {
		return errf("invalid input: %v", err), nil
	}
	// Now unmarshal again to input.
	var input ArxivCategoryFetchLatestArgs
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return errf("failed to unmarshal arguments: %v", err), nil
	}

	if len(input.Category) == 0 {
		return errf("at least one category is required, such as cs.AI"), nil
	}

	// Set default value for CategoryJoinStrategy if empty
	if input.CategoryJoinStrategy == "" {
		input.CategoryJoinStrategy = "AND"
	}

	if input.CategoryJoinStrategy != "AND" && input.CategoryJoinStrategy != "OR" {
		return errf("invalid categoryJoinStrategy: %s (must be AND or OR)", input.CategoryJoinStrategy), nil
	}

	// Build search query for multiple categories
	var searchQuery string
	if len(input.Category) == 1 {
		searchQuery = "cat:" + input.Category[0]
	} else {
		searchQuery = "("
		for i, cat := range input.Category {
			if i > 0 {
				searchQuery += "+" + input.CategoryJoinStrategy + "+"
			}
			searchQuery += "cat:" + cat
		}
		searchQuery += ")"
	}

	// Fetch contents from arXiv API
	url := arxivApiEndpoint + "?search_query=" + searchQuery + "&start=" + fmt.Sprint(input.StartIndex) + "&max_results=" + fmt.Sprint(input.FetchSize) + "&sortBy=submittedDate&sortOrder=descending"
	slog.Debug("Fetching Atom feed from arXiv", "url", url)
	resp, err := http.Get(url)
	if err != nil {
		slog.Warn("failed to fetch from arXiv", "error", err)
		return errf("failed to fetch from arXiv: %v", err), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("failed to read response body", "error", err)
		return errf("failed to read response body: %v", err), nil
	}

	fp := gofeed.NewParser()
	output, err := fp.ParseString(string(body))
	if err != nil {
		slog.Warn("failed to parse feed", "error", err)
		return errf("failed to parse feed: %v", err), nil
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return errf("output failed to marshal: %v", err), nil
	}
	//
	if err := unmarshalAndValidate(outputJSON, t.outputSchema); err != nil {
		return errf("invalid output: %v", err), nil
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(outputJSON)}},
		StructuredContent: output,
	}, nil
}

func ArchiveCategoryFetchLatest(ctx context.Context, req *mcp.CallToolRequest, args ArxivCategoryFetchLatestArgs) (
	*mcp.CallToolResult,
	CategoryFetchLatestOutput,
	error,
) {
	if len(args.Category) == 0 {
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("at least one category is required, such as cs.AI")
	}

	// Set default value for CategoryJoinStrategy if empty
	if args.CategoryJoinStrategy == "" {
		args.CategoryJoinStrategy = "AND"
	}

	if args.CategoryJoinStrategy != "AND" && args.CategoryJoinStrategy != "OR" {
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("invalid categoryJoinStrategy: %s (must be AND or OR)", args.CategoryJoinStrategy)
	}

	// Build search query for multiple categories
	var searchQuery string
	if len(args.Category) == 1 {
		searchQuery = "cat:" + args.Category[0]
	} else {
		searchQuery = "("
		for i, cat := range args.Category {
			if i > 0 {
				searchQuery += "+" + args.CategoryJoinStrategy + "+"
			}
			searchQuery += "cat:" + cat
		}
		searchQuery += ")"
	}

	// Fetch contents from arXiv API
	url := arxivApiEndpoint + "?search_query=" + searchQuery + "&start=" + fmt.Sprint(args.StartIndex) + "&max_results=" + fmt.Sprint(args.FetchSize) + "&sortBy=submittedDate&sortOrder=descending"
	slog.Debug("Fetching Atom feed from arXiv", "url", url)
	resp, err := http.Get(url)
	if err != nil {
		slog.Warn("failed to fetch from arXiv", "error", err)
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("failed to fetch from arXiv: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("failed to read response body", "error", err)
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("failed to read response body: %w", err)
	}

	fp := gofeed.NewParser()
	feed, err := fp.ParseString(string(body))
	if err != nil {
		slog.Warn("failed to parse feed", "error", err)
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("failed to parse feed: %w", err)
	}

	// Format results
	jsonData, err := json.MarshalIndent(feed.Items, "", "  ")
	if err != nil {
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("error marshalling to JSON: %v", err)
	}

	return nil, CategoryFetchLatestOutput{Results: string(jsonData)}, nil
}
