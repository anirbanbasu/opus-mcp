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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// var BuildVersion string = "uninitialised; use -ldflags `-X main.Version=1.0.0`" // Define BuildVersion as a global variable

var startTime time.Time

func uptime() time.Duration {
	return time.Since(startTime)
}

// CORSMiddleware adds CORS headers to responses and handles OPTIONS requests
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set the allowed origin (use specific origins in production, not "*")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, mcp-protocol-version")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass the request to the next handler
		next.ServeHTTP(w, r)
	})
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	responseMap := map[string]any{
		"status":       "ok",
		"name":         "OPUS MCP server (opus-mcp)",
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
	slog.Info("health check responded", "bytes_written", byteN)
}

func runServer(transport_flag string, server_host string, server_port int) {
	ctx := context.Background()
	server := mcp.NewServer(
		&mcp.Implementation{
			Name: "opus-mcp",
			// Use -ldflags to set the version at build time.
			Version: metadata.BuildVersion,
		},
		nil)
	var arxivTool arxiv = arxivImpl{}
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "arxiv_category_fetch_latest",
			Description: "Fetch latest publications from arXiv by category",
		},
		arxivTool.CategoryFetchLatest)

	if transport_flag == "http" {
		// Start HTTP server
		mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcpHandler)
		mux.HandleFunc("/health", healthCheckHandler)
		mux.HandleFunc("/healthz", healthCheckHandler)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close")
			io.WriteString(w, "Use /mcp to access the MCP server. Use /health or /healthz for health checks.")
		})
		handlerWithCORSMiddleware := CORSMiddleware(mux)
		startTime = time.Now()
		// ASCII art: https://patorjk.com/software/taag/#p=display&f=Pagga&t=OPUS+MCP
		fmt.Println(`
░█▀█░█▀█░█░█░█▀▀░░░█▄█░█▀▀░█▀█
░█░█░█▀▀░█░█░▀▀█░░░█░█░█░░░█▀▀
░▀▀▀░▀░░░▀▀▀░▀▀▀░░░▀░▀░▀▀▀░▀░░
		`)
		fmt.Println("BuildVersion: " + metadata.BuildVersion + " BuildTime: " + metadata.BuildTime + ".")
		fmt.Printf("Starting HTTP server on http://%s:%d, press Ctrl+C to stop.\n", server_host, server_port)
		if err := http.ListenAndServe(server_host+":"+fmt.Sprint(server_port), handlerWithCORSMiddleware); err != nil {
			panic(err)
		}
	} else {
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			panic(err)
		}
	}
}

func Serve(transport_flag string, server_host string, server_port int) {
	// Deferred function to recover from a panic
	defer func() {
		if r := recover(); r != nil {
			slog.Error("server crashed,", "error", r)
		}
	}()
	runServer(transport_flag, server_host, server_port)
}
