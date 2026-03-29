package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
	Message  string    `json:"message"`
	Type     string    `json:"type"`
	Priority int       `json:"priority"`
}

func main() {
	limit := flag.Int("limit", 50, "Number of recent insights to fetch (max 500)")
	apiURL := flag.String("api", "", "API base URL (default: http://localhost:8081)")
	raw := flag.Bool("json", false, "Output raw JSON instead of formatted text")
	flag.Parse()

	base := *apiURL
	if base == "" {
		base = os.Getenv("API_URL")
	}
	if base == "" {
		base = "http://localhost:8081"
	}

	url := fmt.Sprintf("%s/api/insights/history?limit=%d", base, *limit)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot reach API at %s: %v\n", base, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: API returned %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	if *raw {
		fmt.Println(string(body))
		return
	}

	var entries []LogEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "error: parsing JSON: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No insights recorded yet.")
		return
	}

	for _, e := range entries {
		fmt.Printf("[%s] (%s, P%d) %s\n",
			e.Timestamp.Format("15:04:05"),
			e.Source,
			e.Priority,
			e.Message,
		)
	}
}
