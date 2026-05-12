package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	k8sclient "github.com/pananagnostou/k8s-lens-mcp/internal/k8s"
	mcpserver "github.com/pananagnostou/k8s-lens-mcp/internal/mcp"
)

func main() {
	k8s, err := k8sclient.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Kubernetes: %v\n", err)
		os.Exit(1)
	}

	s := mcpserver.NewServer(k8s)

	log.Println("k8s-lens-mcp starting on stdio")
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
