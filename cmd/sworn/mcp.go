package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/swornagent/sworn/internal/mcp"
)

func cmdMcp(args []string) int {
	// sworn mcp starts the MCP server. All arguments are ignored — the server
	// reads JSON-RPC 2.0 requests from stdin and writes responses to stdout.
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, `usage: sworn mcp

Starts an MCP 2024-11-05 compliant JSON-RPC 2.0 server over stdio.

The server reads line-delimited JSON-RPC requests from stdin, processes
the initialize/initialized handshake, and responds to tools/list,
resources/list, prompts/list, and tools/call method requests. Tool
implementations are registered by later slices (S08b, S08c).

All diagnostic logs go to stderr; stdout is reserved for the protocol.
`)
		return 0
	}

	server := mcp.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Trap interrupt to shut down cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := server.Run(ctx, os.Stdin, os.Stdout); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "sworn mcp: %v\n", err)
		return 1
	}
	return 0
}