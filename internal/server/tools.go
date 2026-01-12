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
	"opus-mcp/internal/storage"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

const arxivApiEndpoint string = "https://export.arxiv.org/api/query"
const arxivPDFBaseURL string = "https://arxiv.org/pdf/"

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

// ArxivToolHandler is a generic handler that validates input/output and delegates to a handler function
type ArxivToolHandler struct {
	inputSchema  *jsonschema.Resolved
	outputSchema *jsonschema.Resolved
	handlerFunc  func(ctx context.Context, input json.RawMessage) (any, error)
}

// NewArxivToolHandler creates a new tool handler with the given schemas and handler function
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

// Handle is the generic MCP handler that validates input, calls the handler function, and validates output
func (h *ArxivToolHandler) Handle(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Validate input against schema
	if err := unmarshalAndValidate(req.Params.Arguments, h.inputSchema); err != nil {
		return mcp_tool_errorf("invalid input: %v", err), nil
	}

	// Call the handler function
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

// categoryFetchLatest contains the handler function for fetching latest publications by category.
// This function does NOT retry on errors - it returns errors immediately to comply with arXiv API
// terms of use. Rate limiting is enforced to ensure max 1 request per 3 seconds.
// See: https://info.arxiv.org/help/api/tou.html
func categoryFetchLatest(ctx context.Context, input json.RawMessage) (any, error) {
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

// Group represents an arXiv archive or subject group
type Group struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Classification string `json:"classification"`        // broad area (e.g., "Computer Science", "Physics")
	Description    string `json:"description,omitempty"` // optional
}

// Category represents an arXiv category with its metadata
type Category struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Taxonomy represents the complete arXiv category taxonomy
type Taxonomy struct {
	Groups     map[string]Group    `json:"groups"`     // keyed by group code
	Categories map[string]Category `json:"categories"` // keyed by category code
}

// deriveAreaCode converts arXiv area names to their standard codes
// e.g., "Computer Science" → "cs", "Physics" → "physics"
func deriveAreaCode(areaName string) string {
	// Map of known area names to their codes
	areaMap := map[string]string{
		"Computer Science": "cs",
		"Economics":        "econ",
		"Electrical Engineering and Systems Science": "eess",
		"Mathematics":          "math",
		"Physics":              "physics",
		"Quantitative Biology": "q-bio",
		"Quantitative Finance": "q-fin",
		"Statistics":           "stat",
	}

	if code, ok := areaMap[areaName]; ok {
		return code
	}

	// Fallback: convert to lowercase and remove spaces
	return strings.ToLower(strings.ReplaceAll(areaName, " ", "-"))
}

// deriveGroupCode extracts the group code from a category code
// e.g., "cs.AI" → "cs", "astro-ph.CO" → "astro-ph", "hep-ph" → "hep-ph"
func deriveGroupCode(categoryCode string) string {
	// Find the first dot
	if idx := strings.Index(categoryCode, "."); idx > 0 {
		return categoryCode[:idx]
	}
	// No dot found, the entire code is the group
	return categoryCode
}

// fetchCategoryTaxonomy fetches and parses the arXiv category taxonomy from the web.
// Returns a Taxonomy structure with groups and categories in a flattened format.
func fetchCategoryTaxonomy(ctx context.Context, input json.RawMessage) (any, error) {
	const taxonomyURL = "https://arxiv.org/category_taxonomy"

	slog.Info("Fetching and parsing arXiv category taxonomy from", "url", taxonomyURL)
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

	taxonomy := Taxonomy{
		Groups:     make(map[string]Group),
		Categories: make(map[string]Category),
	}

	// Track which groups we've seen to avoid duplicates
	seenGroups := make(map[string]bool)

	// Find all accordion sections (each represents a broad area like "Computer Science")
	doc.Find("h2.accordion-head").Each(func(i int, areaHeading *goquery.Selection) {
		// Get the accordion body (contains all categories for this area)
		accordionBody := areaHeading.Next()

		// Extract area name from the heading and derive area code
		areaName := strings.TrimSpace(areaHeading.Text())
		areaCode := deriveAreaCode(areaName)

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

			// Derive group code from category code
			groupCode := deriveGroupCode(categoryCode)

			// Add group if not seen before
			if !seenGroups[groupCode] {
				seenGroups[groupCode] = true
				// Determine group name
				groupName := areaName
				if groupCode != areaCode {
					// This is a sub-group within the area, try to derive a better name
					// For single-category groups (like "hep-ph"), use the category name
					groupName = categoryName
				}
				taxonomy.Groups[groupCode] = Group{
					Code:           groupCode,
					Name:           groupName,
					Classification: areaName,
				}
			}

			// Add category
			taxonomy.Categories[categoryCode] = Category{
				Code:        categoryCode,
				Name:        categoryName,
				Description: description,
			}
		})
	})

	if len(taxonomy.Categories) == 0 {
		return nil, fmt.Errorf("no categories found in taxonomy")
	}

	return taxonomy, nil
}

// ArxivDownloadPDFArgs defines the input parameters for downloading an arXiv PDF to S3 storage
type ArxivDownloadPDFArgs struct {
	PDFURL                         string `json:"pdfUrl" jsonschema:"The arXiv PDF URL to download (e.g., https://arxiv.org/pdf/2601.05525)"`
	S3Endpoint                     string `json:"s3Endpoint" jsonschema:"S3 server endpoint (e.g., play.min.io:9000 or localhost:9000) (⚠️ Future deprecation warning: passing S3 access credentials as input arguments will be deprecated in the future for better security practices)"`
	S3AccessKey                    string `json:"s3AccessKey" jsonschema:"S3 access key for authentication"`
	S3SecretKey                    string `json:"s3SecretKey" jsonschema:"S3 secret key for authentication"`
	BucketName                     string `json:"bucketName" jsonschema:"Target S3 bucket name where the PDF will be stored"`
	UseSSL                         bool   `json:"useSSL" jsonschema:"Whether to use SSL/TLS for the S3 connection (true for HTTPS, false for HTTP)"`
	SkipSSLCertificateVerification bool   `json:"skipSSLCertificateVerification,omitempty" jsonschema:"Whether to skip certificate verification for the S3 connection. Set to true for self-signed certificates. ⚠️ INSECURE!"`
}

// ArxivDownloadPDFOutput defines the output structure for the PDF download operation
type ArxivDownloadPDFOutput struct {
	Success    bool   `json:"success" jsonschema:"Whether the download and upload operation was successful"`
	Message    string `json:"message" jsonschema:"Status message describing the result of the operation"`
	ObjectName string `json:"objectName,omitempty" jsonschema:"The name/path of the object in the S3 bucket"`
	BucketName string `json:"bucketName,omitempty" jsonschema:"The S3 bucket where the file was stored"`
}

// downloadPDFToMinIO handles downloading an arXiv PDF and uploading it to S3 storage
func downloadPDFToMinIO(ctx context.Context, input json.RawMessage) (any, error) {
	var args ArxivDownloadPDFArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	// Validate the PDF URL
	if !strings.HasPrefix(args.PDFURL, arxivPDFBaseURL) {
		return nil, fmt.Errorf("invalid PDF URL: must start with %s", arxivPDFBaseURL)
	}

	// Extract filename from URL for object naming
	// Example: https://arxiv.org/pdf/2301.00001.pdf -> 2301.00001.pdf
	parts := strings.Split(args.PDFURL, "/")
	filename := parts[len(parts)-1]
	if !strings.HasSuffix(filename, ".pdf") {
		filename = filename + ".pdf"
	}

	// Create object name with arxiv prefix for organisation
	objectName := "arxiv/" + filename

	// Configure S3 client
	config := storage.MinIOConfig{
		Endpoint:           args.S3Endpoint,
		AccessKey:          args.S3AccessKey,
		SecretKey:          args.S3SecretKey,
		UseSSL:             args.UseSSL,
		InsecureSkipVerify: args.SkipSSLCertificateVerification,
	}

	slog.Info("Starting arXiv PDF download to S3 storage",
		"pdf_url", args.PDFURL,
		"bucket", args.BucketName,
		"object", objectName,
		"endpoint", args.S3Endpoint,
		"insecure_tls", args.SkipSSLCertificateVerification)

	// Download and upload to S3
	err := storage.DownloadURLToMinIO(ctx, args.PDFURL, config, args.BucketName, objectName)
	if err != nil {
		return ArxivDownloadPDFOutput{
			Success:    false,
			Message:    fmt.Sprintf("Failed to download and upload PDF: %v", err),
			ObjectName: objectName,
			BucketName: args.BucketName,
		}, nil
	}

	return ArxivDownloadPDFOutput{
		Success:    true,
		Message:    fmt.Sprintf("Successfully downloaded arXiv PDF and uploaded to S3 bucket '%s' as '%s'", args.BucketName, objectName),
		ObjectName: objectName,
		BucketName: args.BucketName,
	}, nil
}
