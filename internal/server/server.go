package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"opus-mcp/internal/metadata"
	"runtime"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// var BuildVersion string = "uninitialised; use -ldflags `-X main.Version=1.0.0`" // Define BuildVersion as a global variable

var serverProcessStartTime time.Time

func uptime() time.Duration {
	return time.Since(serverProcessStartTime)
}

// createCORSMiddleware adds CORS headers to responses and handles OPTIONS requests
func createCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set the allowed origin (use specific origins in production, not "*")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Mcp-Protocol-Version, Mcp-Session-Id")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass the request to the next handler
		next.ServeHTTP(w, r)
	})
}

// createMCPLoggingMiddleware creates an MCP middleware that logs method calls.
func createMCPLoggingMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(
			ctx context.Context,
			method string,
			req mcp.Request,
		) (mcp.Result, error) {
			start := time.Now()
			sessionID := req.GetSession().ID()

			// Log request details.
			slog.Info("(Request) Session: \"" + sessionID + "\" | Method: " + method)

			// Call the actual handler.
			result, err := next(ctx, method, req)

			// Log response details.
			duration := time.Since(start)

			if err != nil {
				slog.Info("(Response) Session: \"" + sessionID + "\" | Method: " + method + " | Status: ERROR | Duration: " + duration.String() + " | Error: " + err.Error())
			} else {
				slog.Info("(Response) Session: \"" + sessionID + "\" | Method: " + method + " | Status: OK | Duration: " + duration.String())
			}

			return result, err
		}
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	responseMap := map[string]any{
		"status":       "ok",
		"name":         metadata.APP_TITLE + " (" + metadata.APP_NAME + ")",
		"buildVersion": metadata.BuildVersion,
		"buildTime":    metadata.BuildTime,
		"uptime":       uptime().String(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
	}
	jsonData, err := json.MarshalIndent(responseMap, "", "    ")
	if err != nil {
		slog.Error("health check JSON marshalling failed", "error", err)
		http.Error(w, "JSON marshalling failed"+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Connection", "close")
	byteN, err := io.Writer.Write(w, jsonData)
	if err != nil {
		slog.Error("health check response writing failed", "error", err)
		http.Error(w, "Response writing failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("health check responded", "bytes_written", byteN)
}

func runServer(transport_flag string, server_host string, server_port int, stateless bool, enableRequestResponseLogging bool) {
	ctx := context.Background()
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:       metadata.APP_NAME,
			Title:      metadata.APP_TITLE,
			WebsiteURL: "https://github.com/anirbanbasu/opus-mcp",
			// Use -ldflags to set the version at build time.
			Version: metadata.BuildVersion,
		},
		&mcp.ServerOptions{
			// Disable logging capability to prevent setLevel errors during initialization
			Capabilities: &mcp.ServerCapabilities{},
		},
	)
	if enableRequestResponseLogging {
		// Add MCP-level logging middleware.
		slog.Info("Server request response logging has been enabled.")
		server.AddReceivingMiddleware(createMCPLoggingMiddleware())
	}
	// Setup all the tools
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "arxiv_category_fetch_latest",
			Description: "Fetch latest publications from arXiv by category",
		},
		ArchiveCategoryFetchLatest)

	var (
		inputSchema = &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"category": {
					Description: "List of unique arXiv categories, e.g., cs.AI",
					Examples:    []any{"cs.AI"},
					Type:        "array",
					MinItems:    jsonschema.Ptr(1),
					Items: &jsonschema.Schema{
						Type: "string",
					},
					UniqueItems: true,
				},
				"startIndex": {
					Description: "The starting index for fetching results (0-based)",
					Type:        "integer",
					Minimum:     jsonschema.Ptr(0.0),
					Default:     json.RawMessage([]byte(`0`)),
				},
				"fetchSize": {
					Description: "Number of results to fetch (min: 1, max: 100)",
					Type:        "integer",
					Minimum:     jsonschema.Ptr(1.0),
					Maximum:     jsonschema.Ptr(100.0),
					Default:     json.RawMessage([]byte(`10`)),
				},
				"categoryJoinStrategy": {
					Description: "Strategy to join multiple categories (AND or OR)",
					Type:        "string",
					Enum:        []any{"AND", "OR"},
					Default:     json.RawMessage([]byte(`"AND"`)),
					Deprecated:  true,
				},
			},
			Required: []string{"category"},
		}
		// TODO: This output schema definition is wrong although this works! Need to fix it later.
		outputSchema = &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		}
		// outputSchema, err = jsonschema.ForType(&gofeed.FeedType, nil)
	)
	arxivWrapper, err := NewArxivWrapper(inputSchema, outputSchema)
	if err != nil {
		slog.Error("failed to create ArxivWrapper", "error", err)
		return
	}
	slog.Info("arxivWrapper created successfully" + arxivWrapper.inputSchema.Schema().ID)

	server.AddTool(&mcp.Tool{
		Name:         "arxiv_category_fetch_latest_manual",
		Description:  "Fetch latest publications from arXiv by category",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}, arxivWrapper.CategoryFetchLatest)

	if transport_flag == "http" {
		// Start HTTP server
		mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, &mcp.StreamableHTTPOptions{Stateless: stateless, JSONResponse: true})
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcpHandler)
		mux.HandleFunc("/health", healthCheckHandler)
		mux.Handle("/healthz", http.RedirectHandler("/health", http.StatusMovedPermanently))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close")
			io.WriteString(w, "Use /mcp to access the MCP server. Use /health or /healthz for health checks.")
		})
		handlerWithCORSMiddleware := createCORSMiddleware(mux)
		serverProcessStartTime = time.Now()
		// ASCII art: https://patorjk.com/software/taag/#p=display&f=Pagga&t=OPUS+MCP
		fmt.Println(`
░█▀█░█▀█░█░█░█▀▀░░░█▄█░█▀▀░█▀█
░█░█░█▀▀░█░█░▀▀█░░░█░█░█░░░█▀▀
░▀▀▀░▀░░░▀▀▀░▀▀▀░░░▀░▀░▀▀▀░▀░░
		`)
		slog.Info("BuildVersion: " + metadata.BuildVersion + " | BuildTime: " + metadata.BuildTime + " | OS: " + runtime.GOOS + " | Architecture: " + runtime.GOARCH)
		slog.Info("Starting HTTP server on http://" + server_host + ":" + fmt.Sprint(server_port) + ", press Ctrl+C to stop.")
		if stateless {
			slog.Info("Server will be set to stateless mode.")
		} else {
			slog.Info("Server will be set to stateful mode.")
		}
		if err := http.ListenAndServe(server_host+":"+fmt.Sprint(server_port), handlerWithCORSMiddleware); err != nil {
			panic(err)
		}
	} else {
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			panic(err)
		}
	}
}

func Serve(transport_flag string, server_host string, server_port int, stateless bool, enableRequestResponseLogging bool) {
	// Deferred function to recover from a panic
	defer func() {
		if r := recover(); r != nil {
			slog.Error("server crashed,", "error", r)
		}
	}()
	runServer(transport_flag, server_host, server_port, stateless, enableRequestResponseLogging)
}
