package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"opus-mcp/internal/metadata"

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

	var (
		inputSchema = &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"category": {
					Description: "Expression of arXiv categories with boolean operators.",
					Examples:    []any{"cs.AI", "cs.LG not cs.CV not cs.RO", "cs.AI + cs.LG - cs.CV", "cs.AI or (cs.LG not cs.CV)"},
					Type:        "string",
					Items: &jsonschema.Schema{
						Type: "string",
					},
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
	categoryHandler, err := NewArxivToolHandler(inputSchema, outputSchema, categoryFetchLatestLogic)
	if err != nil {
		slog.Error("failed to create category fetch handler", "error", err)
		return
	}
	slog.Info("category fetch handler created successfully", "schema_id", categoryHandler.inputSchema.Schema().ID)

	server.AddTool(&mcp.Tool{
		Name:         "arxiv_category_fetch_latest",
		Description:  "Fetch latest publications from arXiv by category. See https://arxiv.org/category_taxonomy for valid categories.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}, categoryHandler.Handle)

	// Category taxonomy tool
	taxonomyInputSchema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{},
	}
	taxonomyOutputSchema := &jsonschema.Schema{
		Type: "object",
		Description: "A nested map structure where each key is a broad area code (e.g., 'cs' for Computer Science) " +
			"and each value is a map of category codes (e.g., 'cs.AI') to their full descriptions.",
		AdditionalProperties: &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: &jsonschema.Schema{
				Type: "string",
			},
		},
	}
	taxonomyHandler, err := NewArxivToolHandler(taxonomyInputSchema, taxonomyOutputSchema, fetchCategoryTaxonomyLogic)
	if err != nil {
		slog.Error("failed to create category taxonomy handler", "error", err)
		return
	}
	slog.Info("category taxonomy handler created successfully")

	server.AddTool(&mcp.Tool{
		Name:         "arxiv_fetch_category_taxonomy",
		Description:  "Fetch the complete arXiv category taxonomy. Returns a nested structure with broad areas (e.g., 'cs') mapping to specific categories (e.g., 'cs.AI') with their descriptions. Data is fetched fresh from https://arxiv.org/category_taxonomy",
		InputSchema:  taxonomyInputSchema,
		OutputSchema: taxonomyOutputSchema,
	}, taxonomyHandler.Handle)

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
			if _, err := io.WriteString(w, "Use /mcp to access the MCP server. Use /health or /healthz for health checks."); err != nil {
				slog.Warn("failed to write response", "error", err)
			}
		})
		handlerWithCORSMiddleware := createCORSMiddleware(mux)
		serverProcessStartTime = time.Now()
		// ASCII art: https://patorjk.com/software/taag/#p=display&f=Pagga&t=OPUS+MCP
		fmt.Println(`
░█▀█░█▀█░█░█░█▀▀░░░█▄█░█▀▀░█▀█
░█░█░█▀▀░█░█░▀▀█░░░█░█░█░░░█▀▀
░▀▀▀░▀░░░▀▀▀░▀▀▀░░░▀░▀░▀▀▀░▀░░
		`)
		slog.Info("Build Version: " + metadata.BuildVersion + " | Build Time: " + metadata.BuildTime + " | OS: " + runtime.GOOS + " | CPU Architecture: " + runtime.GOARCH + " | Stateless Mode: " + fmt.Sprint(stateless))
		slog.Info("Starting HTTP server on http://" + server_host + ":" + fmt.Sprint(server_port) + ", press Ctrl+C to stop")

		httpServer := &http.Server{
			Addr:         server_host + ":" + fmt.Sprint(server_port),
			Handler:      handlerWithCORSMiddleware,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		// Channel to listen for interrupt signals
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		// Channel to notify when server exits
		serverErrors := make(chan error, 1)

		// Start server in a goroutine
		go func() {
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErrors <- err
			}
		}()

		// Wait for interrupt signal or server error
		select {
		case err := <-serverErrors:
			slog.Error("Server failed to start", "error", err)
			panic(err)
		case sig := <-stop:
			slog.Info("Received shutdown signal", "signal", sig)

			// Create a context with 5-second timeout for graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			slog.Info("Shutting down server gracefully (timeout: 5s)...")
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("Server forced to shutdown", "error", err)
				panic(err)
			}
			slog.Info("Server stopped gracefully")
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
