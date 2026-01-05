package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const arxivApiEndpoint string = "http://export.arxiv.org/api/query"

type ArxivCategoryFetchLatestArgs struct {
	Category             []string `json:"category" jsonschema:"The arXiv categories to fetch latest publications from. See taxonomy at https://arxiv.org/category_taxonomy"`
	CategoryJoinStrategy string   `json:"categoryJoinStrategy,omitempty" jsonschema:"Strategy to join multiple categories. Valid values are 'AND' or 'OR'. Defaults to 'AND' if not provided. This has no effect if only one category is provided"`
	StartIndex           uint     `json:"startIndex,omitempty" jsonschema:"The starting index of results to fetch (0-based)"`
	FetchSize            uint     `json:"fetchSize,omitempty" jsonschema:"The number of results to fetch"`
}

type CategoryFetchLatestOutput struct {
	Results string `json:"results" jsonschema:"The latest publications from arXiv in JSON format"`
}

type arxiv interface {
	CategoryFetchLatest(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args ArxivCategoryFetchLatestArgs,
	) (
		*mcp.CallToolResult,
		CategoryFetchLatestOutput,
		error,
	)
}

type arxivImpl struct{}

func (a arxivImpl) CategoryFetchLatest(ctx context.Context, req *mcp.CallToolRequest, args ArxivCategoryFetchLatestArgs) (
	*mcp.CallToolResult,
	CategoryFetchLatestOutput,
	error,
) {
	if len(args.Category) == 0 {
		return nil, CategoryFetchLatestOutput{}, fmt.Errorf("at least one category is required")
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
	slog.Info("Fetching Atom feed from arXiv", "url", url)
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
