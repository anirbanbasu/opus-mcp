package main

import (
	"flag"
	"fmt"

	server "opus-mcp/internal/server"
)

// const (
// 	Reset  = "\033[0m"
// 	Red    = "\033[31m"
// 	Green  = "\033[32m"
// 	Yellow = "\033[33m"
// 	Blue   = "\033[34m"
// )

// func collatzHello() {
// 	var n int
// 	fmt.Println(Yellow + "This is just a Hello world, so far!" + Reset)
// 	fmt.Print(Blue + "Enter a positive integer: " + Reset)
// 	_, err := fmt.Scanln(&n)
// 	if err != nil || n <= 0 {
// 		fmt.Println(Red + "Please enter a valid positive integer." + Reset)
// 		return
// 	}

// 	fmt.Printf(Green+"Collatz sequence for %d: "+Reset, n)
// 	for n != 1 {
// 		fmt.Printf("%d -> ", n)
// 		if n%2 == 0 {
// 			n = n / 2
// 		} else {
// 			n = 3*n + 1
// 		}
// 	}
// 	fmt.Println(1)

// 	fmt.Println(Green + "Collatz sequence completed. Will run the server now, press Ctrl+C to stop." + Reset)
// }

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
	flag.Var(&transport, "transport", "The transport mechanism to use: 'stdio' or 'http'")
	var server_host string = "localhost"
	flag.StringVar(&server_host, "host", "localhost", "The host address for the HTTP server (only relevant if transport is 'http')")
	var server_port int = 8000
	flag.IntVar(&server_port, "port", 8000, "The port for the HTTP server (only relevant if transport is 'http')")
	flag.Parse()
	server.Serve(string(transport), server_host, server_port)
}
