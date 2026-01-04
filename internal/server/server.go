package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"opus-mcp/internal/metadata"
	"opus-mcp/internal/tools"
	"runtime"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// var BuildVersion string = "uninitialised; use -ldflags `-X main.Version=1.0.0`" // Define BuildVersion as a global variable

var startTime time.Time

func uptime() time.Duration {
	return time.Since(startTime)
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
	byteN, err := io.Writer.Write(w, jsonData)
	if err != nil {
		slog.Error("health check response writing failed", "error", err)
		http.Error(w, "Response writing failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("health check responded", "bytes_written", byteN)
}

func run_server(transport_flag string, server_host string, server_port int) {
	ctx := context.Background()
	server := mcp.NewServer(
		&mcp.Implementation{
			Name: "opus-mcp",
			// Use -ldflags to set the version at build time.
			Version: metadata.BuildVersion,
		},
		nil)
	var arxivTool tools.Arxiv = tools.ArxivImpl{}
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "arxiv_category_fetch_latest",
			Description: "Fetch latest publications from arXiv by category",
		},
		arxivTool.CategoryFetchLatest)

	var httpHandler http.Handler
	switch transport_flag {
	case "stdio":
		// do nothing here, handled below
	case "http":
		httpHandler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	default:
		panic("unknown transport flag: " + transport_flag)
	}

	if httpHandler != nil {
		// Start HTTP server
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "Use /mcp to access the MCP server.") })
		mux.Handle("/mcp", httpHandler)
		mux.HandleFunc("/health", healthCheckHandler)
		mux.HandleFunc("/healthz", healthCheckHandler)
		startTime = time.Now()
		// ASCII art: https://patorjk.com/software/taag/#p=display&f=Pagga&t=OPUS+MCP
		fmt.Println(`
░█▀█░█▀█░█░█░█▀▀░░░█▄█░█▀▀░█▀█
░█░█░█▀▀░█░█░▀▀█░░░█░█░█░░░█▀▀
░▀▀▀░▀░░░▀▀▀░▀▀▀░░░▀░▀░▀▀▀░▀░░
		`)
		fmt.Println("BuildVersion: " + metadata.BuildVersion + " BuildTime: " + metadata.BuildTime + ".")
		fmt.Printf("Starting HTTP server on %s:%d, press Ctrl+C to stop.\n", server_host, server_port)
		if err := http.ListenAndServe(server_host+":"+fmt.Sprint(server_port), mux); err != nil {
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
	run_server(transport_flag, server_host, server_port)
}
