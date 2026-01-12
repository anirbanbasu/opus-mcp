package main

import (
	"flag"
	"fmt"

	server "opus-mcp/internal/server"
)

type TransportFlag string

func (t *TransportFlag) String() string {
	return string(*t)
}

func (t *TransportFlag) Set(value string) error {
	if value != "stdio" && value != "http" {
		return fmt.Errorf("must be 'stdio' or 'http'")
	}
	*t = TransportFlag(value)
	return nil
}

func main() {
	var transport TransportFlag = "stdio"
	flag.Var(&transport, "transport", "The transport mechanism to use: 'stdio' or 'http'. The 'http' transport implies streamable HTTP. Note that 'sse' is disbled because it is deprecated.")
	var server_host string = "localhost"
	flag.StringVar(&server_host, "host", "localhost", "The host address for the HTTP server (only relevant if transport is 'http').")
	var server_port int = 8000
	flag.IntVar(&server_port, "port", 8000, "The port for the HTTP server (only relevant if transport is 'http').")
	var stateless bool = false
	flag.BoolVar(&stateless, "stateless", true, "Whether to run the server in stateless mode (only relevant if transport is 'http').")
	var enableRequestResponseLogging bool = false
	flag.BoolVar(&enableRequestResponseLogging, "enableLogging", false, "Whether to enable request and response logging middleware.")
	flag.Parse()
	server.Serve(string(transport), server_host, server_port, stateless, enableRequestResponseLogging)
}
