package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"opus-mcp/internal"
	"opus-mcp/internal/parser"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

const arxivApiEndpoint string = "https://export.arxiv.org/api/query"

// arxivRateLimiter enforces arXiv API rate limit: max 1 request per 3 seconds
// See: https://info.arxiv.org/help/api/tou.html
var arxivRateLimiter = rate.NewLimiter(rate.Every(3*time.Second), 1)

// httpClient is a configured HTTP client with proxy and TLS support
var httpClient = internal.CreateConfiguredHTTPClient()

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

// ArxivToolHandler is a generic handler that validates input/output and delegates to a business logic function
type ArxivToolHandler struct {
	inputSchema  *jsonschema.Resolved
	outputSchema *jsonschema.Resolved
	handlerFunc  func(ctx context.Context, input json.RawMessage) (any, error)
}

// NewArxivToolHandler creates a new tool handler with the given schemas and business logic function
func NewArxivToolHandler(inputSchema, outputSchema *jsonschema.Schema, handlerFunc func(ctx context.Context, input json.RawMessage) (any, error)) (*ArxivToolHandler, error) {
	resIn, err := inputSchema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	resOut, err := outputSchema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	return &ArxivToolHandler{
		inputSchema:  resIn,
		outputSchema: resOut,
		handlerFunc:  handlerFunc,
	}, nil
}

// Handle is the generic MCP handler that validates input, calls the business logic, and validates output
func (h *ArxivToolHandler) Handle(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Validate input against schema
	if err := unmarshalAndValidate(req.Params.Arguments, h.inputSchema); err != nil {
		return mcp_tool_errorf("invalid input: %v", err), nil
	}

	// Call the business logic handler
	result, err := h.handlerFunc(ctx, req.Params.Arguments)
	if err != nil {
		return mcp_tool_errorf("handler error: %v", err), nil
	}

	// Marshal result to JSON
	outputJSON, err := json.Marshal(result)
	if err != nil {
		return mcp_tool_errorf("output failed to marshal: %v", err), nil
	}

	// Validate output against schema
	if err := unmarshalAndValidate(outputJSON, h.outputSchema); err != nil {
		return mcp_tool_errorf("invalid output: %v", err), nil
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(outputJSON)}},
		StructuredContent: result,
	}, nil
}

// categoryFetchLatestLogic contains the business logic for fetching latest publications by category.
// This function does NOT retry on errors - it returns errors immediately to comply with arXiv API
// terms of use. Rate limiting is enforced to ensure max 1 request per 3 seconds.
// See: https://info.arxiv.org/help/api/tou.html
func categoryFetchLatestLogic(ctx context.Context, input json.RawMessage) (any, error) {
	var args ArxivCategoryFetchLatestArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	// Build search query for multiple categories
	searchQuery, err := parser.ParseReconstructCategoryExpression(args.Category)
	if err != nil {
		return nil, fmt.Errorf("failed to parse category expression: %w", err)
	}

	// Enforce rate limit: wait until we're allowed to make a request
	// This ensures compliance with arXiv API terms (max 1 request per 3 seconds)
	if err := arxivRateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}

	// Fetch contents from arXiv API
	url := arxivApiEndpoint + "?search_query=" + searchQuery + "&start=" + fmt.Sprint(args.StartIndex) + "&max_results=" + fmt.Sprint(args.FetchSize) + "&sortBy=submittedDate&sortOrder=descending"
	slog.Info("Fetching Atom feed from arXiv", "url", url)
	resp, err := httpClient.Get(url)
	if err != nil {
		// Return error immediately - no retry logic
		return nil, fmt.Errorf("failed to fetch from arXiv: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	fp := gofeed.NewParser()
	output, err := fp.ParseString(string(body))
	if err != nil {
		// Return error immediately - no retry logic
		return nil, fmt.Errorf("failed to parse feed: %w", err)
	}

	return output, nil
}

// fetchCategoryTaxonomyLogic fetches and parses the arXiv category taxonomy from the web.
// Returns a nested map structure where the top level contains broad areas (e.g., "cs" for Computer Science)
// and each area maps to a map of specific categories (e.g., "cs.AI") to their descriptions.
func fetchCategoryTaxonomyLogic(ctx context.Context, input json.RawMessage) (any, error) {
	const taxonomyURL = "https://arxiv.org/category_taxonomy"

	slog.Info("Fetching arXiv category taxonomy", "url", taxonomyURL)
	resp, err := httpClient.Get(taxonomyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch taxonomy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch taxonomy: HTTP %d", resp.StatusCode)
	}

	// Parse HTML using goquery
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Result structure: map[area]map[category]description
	// e.g., {"cs": {"cs.AI": "Artificial Intelligence description...", ...}}
	taxonomy := make(map[string]map[string]string)

	// Find all accordion sections (each represents a broad area like "Computer Science")
	doc.Find("h2.accordion-head").Each(func(i int, areaHeading *goquery.Selection) {
		// Get the accordion body (contains all categories for this area)
		accordionBody := areaHeading.Next()

		// Create a map for this area's categories
		categories := make(map[string]string)

		// Find all category entries within this area
		accordionBody.Find("h4").Each(func(j int, categoryHeading *goquery.Selection) {
			// Extract category code and name from h4
			// Format: "cs.AI <span>(Artificial Intelligence)</span>"
			fullText := categoryHeading.Text()

			// Split to get the category code (e.g., "cs.AI")
			parts := strings.Fields(fullText)
			if len(parts) < 1 {
				return
			}
			categoryCode := parts[0]

			// Extract the category name from the span
			categoryName := strings.TrimSpace(categoryHeading.Find("span").Text())
			// Remove parentheses if present
			categoryName = strings.Trim(categoryName, "()")

			// Get the description from the next column
			descriptionDiv := categoryHeading.Parent().Next()
			description := strings.TrimSpace(descriptionDiv.Find("p").Text())

			// Combine name and description
			fullDescription := categoryName
			if description != "" {
				fullDescription = categoryName + ": " + description
			}

			categories[categoryCode] = fullDescription
		})

		// Only add if we found categories
		if len(categories) > 0 {
			// Extract area code from first category (e.g., "cs" from "cs.AI")
			for catCode := range categories {
				areaParts := strings.Split(catCode, ".")
				if len(areaParts) > 0 {
					areaCode := areaParts[0]
					taxonomy[areaCode] = categories
					break
				}
			}
		}
	})

	if len(taxonomy) == 0 {
		return nil, fmt.Errorf("no categories found in taxonomy")
	}

	return taxonomy, nil
}

// Example: To add a new tool like AuthorFetchLatest, follow this pattern:
//
// 1. Define the input/output structs:
//    type ArxivAuthorFetchLatestArgs struct {
//        Author     string `json:"author"`
//        StartIndex uint   `json:"startIndex,omitempty"`
//        FetchSize  uint   `json:"fetchSize,omitempty"`
//    }
//
// 2. Create the business logic function:
//    func authorFetchLatestLogic(ctx context.Context, input json.RawMessage) (any, error) {
//        var args ArxivAuthorFetchLatestArgs
//        if err := json.Unmarshal(input, &args); err != nil {
//            return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
//        }
//        // ... implement your logic here
//        return result, nil
//    }
//
// 3. In server.go, create the handler and register it:
//    authorInputSchema := &jsonschema.Schema{ /* ... */ }
//    authorOutputSchema := &jsonschema.Schema{ /* ... */ }
//    authorHandler, err := NewArxivToolHandler(authorInputSchema, authorOutputSchema, authorFetchLatestLogic)
//    server.AddTool(&mcp.Tool{Name: "arxiv_author_fetch_latest", ...}, authorHandler.Handle)
