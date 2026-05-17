// Command math-mcp is an MCP server that exposes accurate math, statistics,
// and financial calculations to LLM clients over stdio. The intent is to give
// LLMs a precise alternative to inferring numeric answers from context.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/matthewjhunter/math-mcp/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-v", "--version":
			fmt.Println(version)
			return
		case "-h", "--help":
			fmt.Fprint(os.Stderr, helpText)
			return
		}
	}

	if err := run(); err != nil {
		log.Fatalf("math-mcp: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := server.New("math-mcp", version)
	return s.Run(ctx, &mcp.StdioTransport{})
}

const helpText = `math-mcp — MCP server for accurate math and financial calculations.

Usage:
  math-mcp           Run the MCP server over stdio (default; the standard MCP transport)
  math-mcp -v        Print version
  math-mcp -h        Show this help

Register with Claude Code (user scope):
  claude mcp add math-mcp -s user -- math-mcp`
