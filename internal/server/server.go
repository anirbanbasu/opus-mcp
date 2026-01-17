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
	"reflect"
	"runtime"
	"syscall"
	"time"

	"opus-mcp/internal/metadata"
	"opus-mcp/internal/storage"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sethvargo/go-envconfig"
)

var serverProcessStartTime time.Time

// globalS3Config is the S3 configuration loaded at server startup
var globalS3Config *storage.S3Config

func uptime() time.Duration {
	return time.Since(serverProcessStartTime)
}

// LoadS3Config loads S3 configuration from environment variables
func LoadS3Config() (*storage.S3Config, error) {
	ctx := context.Background()
	var config storage.S3Config
	if err := envconfig.Process(ctx, &config); err != nil {
		slog.Error("Failed to process S3 configuration from environment", "error", err)
		return nil, err
	}

	if config.InsecureSkipVerify {
		slog.Warn("⚠️  S3 TLS certificate verification is DISABLED - this is insecure!")
	}

	slog.Info("S3 configuration loaded from environment variables",
		"endpoint", config.Endpoint,
		"use_ssl", config.UseSSL,
		"insecure_skip_verify", config.InsecureSkipVerify)

	return &config, nil
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

func addMCPTools(server *mcp.Server) error {
	categoryFetchLatestInputSchema := &jsonschema.Schema{
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
				Minimum:     jsonschema.Ptr(float64(0)),
				Default:     json.RawMessage([]byte(`0`)),
			},
			"fetchSize": {
				Description: "Number of results to fetch (min: 1, max: 100)",
				Type:        "integer",
				Minimum:     jsonschema.Ptr(float64(1)),
				Maximum:     jsonschema.Ptr(float64(100)),
				Default:     json.RawMessage([]byte(`10`)),
			},
		},
		Required: []string{"category"},
	}

	// Generate output schema from gofeed.Feed structure using reflection
	// Handle circular references by providing simplified schemas for problematic types
	categoryFetchLatestOutputSchema, err := jsonschema.ForType(reflect.TypeFor[gofeed.Feed](), &jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			// Break the cycle in ITunesCategory
			reflect.TypeFor[ext.ITunesCategory](): {
				Type:        "object",
				Description: "iTunes category (circular reference simplified)",
				Properties: map[string]*jsonschema.Schema{
					"text":        {Type: "string"},
					"subcategory": {Type: "object", Description: "Nested subcategory"},
				},
			},
			// Break the cycle in Extension
			reflect.TypeFor[ext.Extension](): {
				Type:        "object",
				Description: "Generic extension (circular reference simplified)",
				// Allow any type for extensions (nested objects, arrays, strings, etc.)
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to reflect output schema from gofeed.Feed: %w", err)
	}

	categoryFetchLatestHandler, err := NewArxivToolHandler(categoryFetchLatestInputSchema, categoryFetchLatestOutputSchema, categoryFetchLatest)
	if err != nil {
		return fmt.Errorf("failed to create category fetch latest handler: %w", err)
	}
	slog.Info("category fetch handler created successfully")

	server.AddTool(&mcp.Tool{
		Name:         "arxiv_category_fetch_latest",
		Description:  "Fetch latest publications from arXiv by category. See https://arxiv.org/category_taxonomy for valid categories.",
		InputSchema:  categoryFetchLatestInputSchema,
		OutputSchema: categoryFetchLatestOutputSchema,
	}, categoryFetchLatestHandler.Handle)

	// Category taxonomy tool
	taxonomyInputSchema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{},
	}
	// Generate output schema from Taxonomy structure using reflection
	taxonomyOutputSchema, err := jsonschema.ForType(reflect.TypeFor[Taxonomy](), &jsonschema.ForOptions{})
	if err != nil {
		return fmt.Errorf("failed to reflect output schema from Taxonomy: %w", err)
	}
	taxonomyHandler, err := NewArxivToolHandler(taxonomyInputSchema, taxonomyOutputSchema, fetchCategoryTaxonomy)
	if err != nil {
		return fmt.Errorf("failed to create category taxonomy handler: %w", err)
	}
	slog.Info("category taxonomy handler created successfully")

	server.AddTool(&mcp.Tool{
		Name:         "arxiv_get_category_taxonomy",
		Description:  "Fetch the complete arXiv category taxonomy. Returns a nested structure with broad areas (e.g., 'cs') mapping to specific categories (e.g., 'cs.AI') with their descriptions. Data is fetched fresh from https://arxiv.org/category_taxonomy",
		InputSchema:  taxonomyInputSchema,
		OutputSchema: taxonomyOutputSchema,
	}, taxonomyHandler.Handle)

	// ArXiv PDF download to S3 tool
	if globalS3Config != nil {
		downloadPDFInputSchema, err := jsonschema.ForType(reflect.TypeFor[ArxivDownloadPDFArgs](), &jsonschema.ForOptions{})
		if err != nil {
			return fmt.Errorf("failed to reflect input schema from ArxivDownloadPDFArgs: %w", err)
		}
		downloadPDFOutputSchema, err := jsonschema.ForType(reflect.TypeFor[ArxivDownloadPDFOutput](), &jsonschema.ForOptions{})
		if err != nil {
			return fmt.Errorf("failed to reflect output schema from ArxivDownloadPDFOutput: %w", err)
		}
		downloadPDFHandler, err := NewArxivToolHandler(downloadPDFInputSchema, downloadPDFOutputSchema, downloadPDFToS3)
		if err != nil {
			return fmt.Errorf("failed to create arXiv PDF download handler: %w", err)
		}
		slog.Info("arXiv PDF download handler created successfully")

		server.AddTool(&mcp.Tool{
			Name:         "arxiv_download_pdf",
			Description:  "Download an arXiv PDF from a URL and upload it to a S3 bucket, e.g., over MinIO. Requires S3 credentials. The PDF will be stored in the 'arxiv/' prefix within the '" + metadata.S3_ARTICLES_BUCKET + "' bucket.",
			InputSchema:  downloadPDFInputSchema,
			OutputSchema: downloadPDFOutputSchema,
		}, downloadPDFHandler.Handle)
	} else {
		slog.Info("Skipping arXiv PDF download tool addition - S3 configuration not available")
	}

	return nil
}

func runServer(transport_flag string, server_host string, server_port int, enableRequestResponseLogging bool) {
	// Load S3 configuration from environment variables at startup
	var err error
	globalS3Config, err = LoadS3Config()
	if err != nil {
		slog.Warn("S3 configuration not available - S3-dependent tools will be disabled", "error", err)
		slog.Warn("To enable S3 features, set: OPUS_MCP_S3_ENDPOINT, OPUS_MCP_S3_ACCESS_KEY, OPUS_MCP_S3_SECRET_KEY")
		globalS3Config = nil
	}

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

	// Add MCP tools
	if err := addMCPTools(server); err != nil {
		slog.Error("failed to add MCP tools", "error", err)
		// Should we panic here?
	}
	slog.Info("MCP tools added successfully")

	if transport_flag == "http" {
		// Start HTTP server -- should the server have a stateless or stateful option for logging per MCP client ID, at least?
		mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
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
		slog.Info("Build Version: " + metadata.BuildVersion + " | Build Time: " + metadata.BuildTime + " | OS: " + runtime.GOOS + " | CPU Architecture: " + runtime.GOARCH)
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

func Serve(transport_flag string, server_host string, server_port int, enableRequestResponseLogging bool) {
	// Deferred function to recover from a panic
	defer func() {
		if r := recover(); r != nil {
			slog.Error("server crashed,", "error", r)
		}
	}()
	runServer(transport_flag, server_host, server_port, enableRequestResponseLogging)
}
