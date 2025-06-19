package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// LokiResult represents the structure of Loki query results
type LokiResult struct {
	Status string   `json:"status"`
	Data   LokiData `json:"data"`
	Error  string   `json:"error,omitempty"`
}

// LokiData represents the data portion of Loki results
type LokiData struct {
	ResultType string      `json:"resultType"`
	Result     []LokiEntry `json:"result"`
}

// LokiEntry represents a single log entry from Loki
type LokiEntry struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"` // [timestamp, log line]
}

// SSEEvent represents an event to be sent via SSE
type SSEEvent struct {
	Type      string `json:"type"`
	Query     string `json:"query"`
	Timestamp string `json:"timestamp"`
	Results   any    `json:"results"`
}

// Environment variable name for Loki URL
const EnvLokiURL = "LOKI_URL"

// Environment variable name for Loki Organization ID
const EnvLokiOrgID = "LOKI_ORG_ID"

// Environment variable name for Loki Username
const EnvLokiUsername = "LOKI_USERNAME"

// Environment variable name for Loki Password
const EnvLokiPassword = "LOKI_PASSWORD"

// Environment variable name for Loki Token
const EnvLokiToken = "LOKI_TOKEN"

// Default Loki URL when environment variable is not set
const DefaultLokiURL = "http://localhost:3100"

// NewLokiQueryTool creates and returns a tool for querying Grafana Loki
func NewLokiQueryTool() mcp.Tool {
	// Get Loki URL from environment variable or use default
	lokiURL := os.Getenv(EnvLokiURL)
	if lokiURL == "" {
		lokiURL = DefaultLokiURL
	}

	// Get Loki Org ID from environment variable if set
	orgID := os.Getenv(EnvLokiOrgID)

	// Get authentication parameters from environment variables if set
	username := os.Getenv(EnvLokiUsername)
	password := os.Getenv(EnvLokiPassword)
	token := os.Getenv(EnvLokiToken)

	return mcp.NewTool("loki_query",
		mcp.WithDescription("Run a query against Grafana Loki"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("LogQL query string"),
		),
		mcp.WithString("url",
			mcp.Description(fmt.Sprintf("Loki server URL (default: %s from %s env var)", lokiURL, EnvLokiURL)),
			mcp.DefaultString(lokiURL),
		),
		mcp.WithString("username",
			mcp.Description(fmt.Sprintf("Username for basic authentication (default: %s from %s env var)", username, EnvLokiUsername)),
		),
		mcp.WithString("password",
			mcp.Description(fmt.Sprintf("Password for basic authentication (default: %s from %s env var)", password, EnvLokiPassword)),
		),
		mcp.WithString("token",
			mcp.Description(fmt.Sprintf("Bearer token for authentication (default: %s from %s env var)", token, EnvLokiToken)),
		),
		mcp.WithString("start",
			mcp.Description("Start time for the query (default: 1h ago)"),
		),
		mcp.WithString("end",
			mcp.Description("End time for the query (default: now)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries to return (default: 100)"),
		),
		mcp.WithString("org",
			mcp.Description(fmt.Sprintf("Organization ID for the query (default: %s from %s env var)", orgID, EnvLokiOrgID)),
		),
	)
}

// HandleLokiQuery handles Loki query tool requests
func HandleLokiQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract parameters
	args := request.GetArguments()
	queryString := args["query"].(string)

	// Get Loki URL from request arguments, if not present check environment
	var lokiURL string
	if urlArg, ok := args["url"].(string); ok && urlArg != "" {
		lokiURL = urlArg
	} else {
		// Fallback to environment variable
		lokiURL = os.Getenv(EnvLokiURL)
		if lokiURL == "" {
			lokiURL = DefaultLokiURL
		}
	}

	// Extract authentication parameters
	var username, password, token, orgID string
	if usernameArg, ok := args["username"].(string); ok && usernameArg != "" {
		username = usernameArg
	} else {
		// Fallback to environment variable
		username = os.Getenv(EnvLokiUsername)
	}
	if passwordArg, ok := args["password"].(string); ok && passwordArg != "" {
		password = passwordArg
	} else {
		// Fallback to environment variable
		password = os.Getenv(EnvLokiPassword)
	}
	if tokenArg, ok := args["token"].(string); ok && tokenArg != "" {
		token = tokenArg
	} else {
		// Fallback to environment variable
		token = os.Getenv(EnvLokiToken)
	}
	if orgIDArg, ok := args["org"].(string); ok && orgIDArg != "" {
		orgID = orgIDArg
	} else {
		// Fallback to environment variable
		orgID = os.Getenv(EnvLokiOrgID)
	}

	// Set defaults for optional parameters
	start := time.Now().Add(-1 * time.Hour).Unix()
	end := time.Now().Unix()
	limit := 100

	// Override defaults if parameters are provided
	if startStr, ok := args["start"].(string); ok && startStr != "" {
		startTime, err := parseTime(startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid start time: %v", err)
		}
		start = startTime.Unix()
	}

	if endStr, ok := args["end"].(string); ok && endStr != "" {
		endTime, err := parseTime(endStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end time: %v", err)
		}
		end = endTime.Unix()
	}

	if limitVal, ok := args["limit"].(float64); ok {
		limit = int(limitVal)
	}

	// Build query URL
	queryURL, err := buildLokiQueryURL(lokiURL, queryString, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to build query URL: %v", err)
	}

	// Execute query with authentication
	result, err := executeLokiQuery(ctx, queryURL, username, password, token, orgID)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %v", err)
	}

	// Format results
	formattedResult, err := formatLokiResults(result)
	if err != nil {
		return nil, fmt.Errorf("failed to format results: %v", err)
	}

	// Broadcast results to SSE clients if available
	broadcastQueryResults(ctx, queryString, result)

	return mcp.NewToolResultText(formattedResult), nil
}

// broadcastQueryResults sends the query results to all connected SSE clients
func broadcastQueryResults(ctx context.Context, queryString string, result *LokiResult) {
	// In the simplified approach, we don't explicitly broadcast events
	// The SSE server automatically handles tool calls through the MCPServer

	// This function is kept as a placeholder for future enhancements
	// or if you decide to implement custom broadcasting later
}

// parseTime parses a time string in various formats
func parseTime(timeStr string) (time.Time, error) {
	// Handle "now" keyword
	if timeStr == "now" {
		return time.Now(), nil
	}

	// Handle relative time strings like "-1h", "-30m"
	if len(timeStr) > 0 && timeStr[0] == '-' {
		duration, err := time.ParseDuration(timeStr)
		if err == nil {
			return time.Now().Add(duration), nil
		}
	}

	// Try parsing as RFC3339
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}

	// Try other common formats
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		t, err := time.Parse(format, timeStr)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported time format: %s", timeStr)
}

// buildLokiQueryURL constructs the Loki query URL
func buildLokiQueryURL(baseURL, query string, start, end int64, limit int) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// Add path for Loki query API only if not already included
	if !strings.Contains(u.Path, "loki/api/v1") {
		if u.Path == "" || u.Path == "/" {
			u.Path = "/loki/api/v1/query_range"
		} else {
			u.Path = fmt.Sprintf("%s/loki/api/v1/query_range", u.Path)
		}
	} else {
		// If path already contains loki/api/v1, just append query_range if not present
		if !strings.HasSuffix(u.Path, "query_range") {
			u.Path = fmt.Sprintf("%s/query_range", u.Path)
		}
	}

	// Add query parameters
	q := u.Query()
	q.Set("query", query)
	q.Set("start", fmt.Sprintf("%d", start))
	q.Set("end", fmt.Sprintf("%d", end))
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// executeLokiQuery sends the HTTP request to Loki
func executeLokiQuery(ctx context.Context, queryURL string, username, password, token, orgID string) (*LokiResult, error) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication if provided
	if token != "" {
		// Bearer token authentication
		req.Header.Add("Authorization", "Bearer "+token)
	} else if username != "" || password != "" {
		// Basic authentication
		req.SetBasicAuth(username, password)
	}

	// Add orgid if provided
	if orgID != "" {
		req.Header.Add("X-Scope-OrgID", orgID)
	}

	// Execute request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result LokiResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// Check for Loki errors
	if result.Status == "error" {
		return nil, fmt.Errorf("loki error: %s", result.Error)
	}

	return &result, nil
}

// formatLokiResults formats the Loki query results into a readable string
func formatLokiResults(result *LokiResult) (string, error) {
	if len(result.Data.Result) == 0 {
		return "No logs found matching the query", nil
	}

	var output string
	output = fmt.Sprintf("Found %d streams:\n\n", len(result.Data.Result))

	for i, entry := range result.Data.Result {
		// Format stream labels
		streamInfo := "Stream "
		if len(entry.Stream) > 0 {
			streamInfo += "("
			first := true
			for k, v := range entry.Stream {
				if !first {
					streamInfo += ", "
				}
				streamInfo += fmt.Sprintf("%s=%s", k, v)
				first = false
			}
			streamInfo += ")"
		}

		output += fmt.Sprintf("%s %d:\n", streamInfo, i+1)

		// Format log entries
		for _, val := range entry.Values {
			if len(val) >= 2 {
				// Parse timestamp
				ts, err := strconv.ParseFloat(val[0], 64)
				if err == nil {
					// Convert to time - Loki returns timestamps in nanoseconds already
					timestamp := time.Unix(0, int64(ts))
					output += fmt.Sprintf("[%s] %s\n", timestamp.Format(time.RFC3339), val[1])
				} else {
					output += fmt.Sprintf("[%s] %s\n", val[0], val[1])
				}
			}
		}
		output += "\n"
	}

	return output, nil
}
