package main

import (
	"fmt"

	server "github.com/anirbanbasu/opus-mcp/internal"
)

const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
)

func main() {
	var n int
	// ASCII art: https://patorjk.com/software/taag/#p=display&f=Pagga&t=OPUS+MCP
	fmt.Println(Yellow + "This is just a Hello world, so far!" + Reset)
	fmt.Print(Blue + "Enter a positive integer: " + Reset)
	_, err := fmt.Scanln(&n)
	if err != nil || n <= 0 {
		fmt.Println(Red + "Please enter a valid positive integer." + Reset)
		return
	}

	fmt.Printf(Green+"Collatz sequence for %d: "+Reset, n)
	for n != 1 {
		fmt.Printf("%d -> ", n)
		if n%2 == 0 {
			n = n / 2
		} else {
			n = 3*n + 1
		}
	}
	fmt.Println(1)

	fmt.Println(Green + "Collatz sequence completed. Will run the server now, press Ctrl+C to stop." + Reset)
	server.Serve()
}
