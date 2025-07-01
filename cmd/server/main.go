package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/scottlepp/loki-mcp/internal/handlers"
)

const (
	version = "0.1.0"
)

func main() {
	// Create a new MCP server
	s := server.NewMCPServer(
		"Loki MCP Server",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Add Loki query tool
	lokiQueryTool := handlers.NewLokiQueryTool()
	s.AddTool(lokiQueryTool, handlers.HandleLokiQuery)

	// Add Loki label names tool
	lokiLabelNamesTool := handlers.NewLokiLabelNamesTool()
	s.AddTool(lokiLabelNamesTool, handlers.HandleLokiLabelNames)

	// Add Loki label values tool
	lokiLabelValuesTool := handlers.NewLokiLabelValuesTool()
	s.AddTool(lokiLabelValuesTool, handlers.HandleLokiLabelValues)

	// Get port from environment variable or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create SSE server for legacy SSE connections
	sseServer := server.NewSSEServer(s,
		server.WithSSEEndpoint("/sse"),
		server.WithMessageEndpoint("/mcp"),
	)

	// Create Streamable HTTP server
	streamableServer := server.NewStreamableHTTPServer(s)

	// Create a multiplexer to handle both protocols on the same port
	mux := http.NewServeMux()

	// Register SSE endpoints (legacy support)
	mux.Handle("/sse", sseServer) // SSE event stream
	mux.Handle("/mcp", sseServer) // SSE message endpoint

	// Register Streamable HTTP endpoint
	mux.Handle("/stream", streamableServer) // Streamable HTTP endpoint

	// Create a channel to handle shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start unified HTTP server
	go func() {
		addr := fmt.Sprintf(":%s", port)
		log.Printf("Starting unified MCP server on http://localhost%s", addr)
		log.Printf("SSE Endpoint (legacy): http://localhost%s/sse", addr)
		log.Printf("SSE Message Endpoint: http://localhost%s/mcp", addr)
		log.Printf("Streamable HTTP Endpoint: http://localhost%s/stream", addr)

		if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// For backward compatibility, also serve via stdio
	go func() {
		log.Println("Starting stdio server")
		if err := server.ServeStdio(s); err != nil {
			log.Printf("Stdio server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-stop
	log.Println("Shutting down servers...")
}
