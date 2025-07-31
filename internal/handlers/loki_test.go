package handlers

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestFormatLokiResults_TimestampParsing tests that timestamps from Loki are correctly parsed
// This test specifically addresses the bug where timestamps were showing as year 2262
// due to incorrect conversion from nanoseconds to time objects.
func TestFormatLokiResults_TimestampParsing(t *testing.T) {
	// Test case with known timestamp
	// Using a specific timestamp: 2024-01-15T10:30:45Z = 1705312245000000000 nanoseconds
	timestampStr := "1705312245000000000" // This represents the nanosecond timestamp

	// Create a sample Loki result with the test timestamp
	result := &LokiResult{
		Status: "success",
		Data: LokiData{
			ResultType: "streams",
			Result: []LokiEntry{
				{
					Stream: map[string]string{
						"job":   "test-job",
						"level": "info",
					},
					Values: [][]string{
						{timestampStr, "Test log message"},
					},
				},
			},
		},
	}

	// Format the results
	output, err := formatLokiResults(result, "raw")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Verify the output contains the correct timestamp
	// Note: The timestamp will be formatted in local timezone, so we check for the date part
	if !strings.Contains(output, "2024-01-15T") {
		t.Errorf("Expected output to contain date '2024-01-15T', but got:\n%s", output)
	}

	// Verify it doesn't contain the year 2262 (the bug we fixed)
	if strings.Contains(output, "2262") {
		t.Errorf("Output contains year 2262, indicating timestamp parsing bug is present:\n%s", output)
	}

	// Verify it contains the expected log message
	if !strings.Contains(output, "Test log message") {
		t.Errorf("Expected output to contain 'Test log message', but got:\n%s", output)
	}

	// Verify it contains the stream information
	if !strings.Contains(output, "job=test-job") {
		t.Errorf("Expected output to contain stream info 'job=test-job', but got:\n%s", output)
	}
}

// TestFormatLokiResults_MultipleTimestamps tests parsing of multiple log entries with different timestamps
func TestFormatLokiResults_MultipleTimestamps(t *testing.T) {
	result := &LokiResult{
		Status: "success",
		Data: LokiData{
			ResultType: "streams",
			Result: []LokiEntry{
				{
					Stream: map[string]string{
						"job": "test-job",
					},
					Values: [][]string{
						{"1705312245000000000", "First log message"},  // 2024-01-15T10:30:45Z
						{"1705312260000000000", "Second log message"}, // 2024-01-15T10:31:00Z
						{"1705312275000000000", "Third log message"},  // 2024-01-15T10:31:15Z
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "raw")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Check that all timestamps are in 2024, not 2262
	if strings.Contains(output, "2262") {
		t.Errorf("Output contains year 2262, indicating timestamp parsing bug:\n%s", output)
	}

	// All timestamps should be in 2024
	occurrences := strings.Count(output, "2024")
	if occurrences < 3 {
		t.Errorf("Expected at least 3 occurrences of '2024' in output, but found %d:\n%s", occurrences, output)
	}
}

// TestFormatLokiResults_InvalidTimestamp tests handling of invalid timestamp strings
func TestFormatLokiResults_InvalidTimestamp(t *testing.T) {
	result := &LokiResult{
		Status: "success",
		Data: LokiData{
			ResultType: "streams",
			Result: []LokiEntry{
				{
					Stream: map[string]string{
						"job": "test-job",
					},
					Values: [][]string{
						{"invalid-timestamp", "Log with invalid timestamp"},
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "text")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Should contain the original invalid timestamp as fallback
	if !strings.Contains(output, "[invalid-timestamp]") {
		t.Errorf("Expected output to contain '[invalid-timestamp]' as fallback, but got:\n%s", output)
	}

	// Should still contain the log message
	if !strings.Contains(output, "Log with invalid timestamp") {
		t.Errorf("Expected output to contain log message, but got:\n%s", output)
	}
}

// TestFormatLokiResults_EmptyResult tests handling of empty results
func TestFormatLokiResults_EmptyResult(t *testing.T) {
	result := &LokiResult{
		Status: "success",
		Data: LokiData{
			ResultType: "streams",
			Result:     []LokiEntry{},
		},
	}

	output, err := formatLokiResults(result, "raw")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	expected := "No logs found matching the query"
	if output != expected {
		t.Errorf("Expected output '%s', but got '%s'", expected, output)
	}
}

// TestFormatLokiResults_RecentTimestamp tests with a very recent timestamp to ensure current dates work
func TestFormatLokiResults_RecentTimestamp(t *testing.T) {
	// Use current time
	now := time.Now().UTC()
	timestampNanos := now.UnixNano()
	timestampStr := strconv.FormatInt(timestampNanos, 10)

	result := &LokiResult{
		Status: "success",
		Data: LokiData{
			ResultType: "streams",
			Result: []LokiEntry{
				{
					Stream: map[string]string{
						"job": "recent-test",
					},
					Values: [][]string{
						{timestampStr, "Recent log message"},
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "raw")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Should contain current year, not 2262
	currentYear := now.Format("2006")
	if !strings.Contains(output, currentYear) {
		t.Errorf("Expected output to contain current year %s, but got:\n%s", currentYear, output)
	}

	if strings.Contains(output, "2262") {
		t.Errorf("Output contains year 2262, indicating timestamp parsing bug:\n%s", output)
	}
}

// TestFormatLokiResults_NoYear2262Bug is a regression test for the specific bug reported in issue #3
// This test ensures that timestamps never show year 2262 due to incorrect nanosecond conversion
func TestFormatLokiResults_NoYear2262Bug(t *testing.T) {
	// This test uses a variety of realistic nanosecond timestamps
	testCases := []struct {
		name         string
		timestampNs  string
		expectedYear string
	}{
		{
			name:         "Current timestamp",
			timestampNs:  "1705312245000000000", // 2024-01-15T10:30:45Z
			expectedYear: "2024",
		},
		{
			name:         "Recent timestamp",
			timestampNs:  "1700000000000000000", // 2023-11-14T22:13:20Z
			expectedYear: "2023",
		},
		{
			name:         "Future timestamp",
			timestampNs:  "1800000000000000000", // 2027-01-11T02:13:20Z
			expectedYear: "2027",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &LokiResult{
				Status: "success",
				Data: LokiData{
					ResultType: "streams",
					Result: []LokiEntry{
						{
							Stream: map[string]string{
								"job": "regression-test",
							},
							Values: [][]string{
								{tc.timestampNs, "Test log message"},
							},
						},
					},
				},
			}

			output, err := formatLokiResults(result, "raw")
			if err != nil {
				t.Fatalf("formatLokiResults failed: %v", err)
			}

			// The main regression check: ensure we never see year 2262
			if strings.Contains(output, "2262") {
				t.Errorf("REGRESSION: Output contains year 2262, the original bug is present:\n%s", output)
			}

			// Verify we see the expected year instead
			if !strings.Contains(output, tc.expectedYear) {
				t.Errorf("Expected output to contain year %s, but got:\n%s", tc.expectedYear, output)
			}
		})
	}
}
