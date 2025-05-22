package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Request represents a JSON-RPC request
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// Response represents a JSON-RPC response
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject represents a JSON-RPC error object
type ErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolResult represents the result of a call_tool request
type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError"`
}

// ContentItem represents a content item in a tool result
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	// Start the MCP server in a separate process
	cmd := exec.Command("go", "run", "./cmd/server/main.go")

	// Connect stdin and stdout to the MCP server
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	// Set up stderr to be displayed
	cmd.Stderr = os.Stderr

	// Start the server
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Create a reader for the stdout
	reader := bufio.NewReader(stdout)

	var req Request

	// Process commands
	switch os.Args[1] {
	case "loki_query":
		if len(os.Args) < 3 {
			fmt.Println("Usage: client loki_query [url] <query> [start] [end] [limit]")
			fmt.Println("Examples:")
			fmt.Println("  client loki_query \"{job=\\\"varlogs\\\"}\"")
			fmt.Println("  client loki_query http://localhost:3100 \"{job=\\\"varlogs\\\"}\"")
			fmt.Println("  client loki_query \"{job=\\\"varlogs\\\"}\" \"-1h\" \"now\" 100")
			os.Exit(1)
		}

		var lokiURL, query, start, end string
		var limit float64

		// Check if the first argument is a URL or a query
		if strings.HasPrefix(os.Args[2], "http") {
			// First arg is URL, second is query
			if len(os.Args) < 4 {
				fmt.Println("Error: When providing a URL, you must also provide a query")
				os.Exit(1)
			}
			lokiURL = os.Args[2]
			query = os.Args[3]
			argOffset := 4

			// Optional parameters with URL
			if len(os.Args) > argOffset {
				start = os.Args[argOffset]
			}

			if len(os.Args) > argOffset+1 {
				end = os.Args[argOffset+1]
			}

			if len(os.Args) > argOffset+2 {
				limitVal, err := strconv.ParseFloat(os.Args[argOffset+2], 64)
				if err != nil {
					log.Fatalf("Invalid number for limit: %v", err)
				}
				limit = limitVal
			}
		} else {
			// First arg is the query (URL comes from environment)
			query = os.Args[2]
			argOffset := 3

			// Optional parameters without URL
			if len(os.Args) > argOffset {
				start = os.Args[argOffset]
			}

			if len(os.Args) > argOffset+1 {
				end = os.Args[argOffset+1]
			}

			if len(os.Args) > argOffset+2 {
				limitVal, err := strconv.ParseFloat(os.Args[argOffset+2], 64)
				if err != nil {
					log.Fatalf("Invalid number for limit: %v", err)
				}
				limit = limitVal
			}
		}

		// Create the Loki query request
		req = createLokiQueryRequest(lokiURL, query, start, end, limit)

	default:
		showUsage()
		os.Exit(1)
	}

	// Marshal the request to JSON
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}

	fmt.Printf("Sending request: %s\n", string(reqJSON))

	// Send the request to the server
	_, err = stdin.Write(append(reqJSON, '\n'))
	if err != nil {
		log.Fatalf("Failed to send request: %v", err)
	}

	// Read the response from the server
	respJSON, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		log.Fatalf("Failed to read response: %v", err)
	}

	fmt.Printf("Received response: %s\n", string(respJSON))

	// Unmarshal the response
	var resp Response
	if err := json.Unmarshal(respJSON, &resp); err != nil {
		log.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Check for errors
	if resp.Error != nil {
		log.Fatalf("Error from server: %s", resp.Error.Message)
	}

	// Unmarshal the result
	var result ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		log.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Process the result
	if result.IsError {
		fmt.Printf("Error: %s\n", result.Content[0].Text)
	} else {
		// Display the result
		for _, item := range result.Content {
			fmt.Println(item.Text)
		}
	}

	// Terminate the server
	if err := cmd.Process.Kill(); err != nil {
		log.Printf("Failed to kill server process: %v", err)
	}
}

func showUsage() {
	fmt.Println("Usage:")
	fmt.Println("  client loki_query [url] <query> [start] [end] [limit]")
	fmt.Println("    Examples:")
	fmt.Println("      client loki_query \"{job=\\\"varlogs\\\"}\"")
	fmt.Println("      client loki_query http://localhost:3100 \"{job=\\\"varlogs\\\"}\"")
	fmt.Println("      client loki_query \"{job=\\\"varlogs\\\"}\" \"-1h\" \"now\" 100")
}

func createLokiQueryRequest(url, query, start, end string, limit float64) Request {
	// Create arguments map
	args := map[string]any{
		"query": query,
	}

	// Add URL parameter if provided, otherwise, let the server use the environment variable
	if url != "" {
		args["url"] = url
	}

	// Add optional parameters if provided
	if start != "" {
		args["start"] = start
	}

	if end != "" {
		args["end"] = end
	}

	if limit > 0 {
		args["limit"] = limit
	}

	return Request{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "loki_query",
			"arguments": args,
		},
	}
}

func getOperationSymbol(operation string) string {
	switch operation {
	case "add":
		return "+"
	case "subtract":
		return "-"
	case "multiply":
		return "*"
	case "divide":
		return "/"
	default:
		return operation
	}
}
