package hams

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// storage for callsigns
var (
	callSigns []string
	mu        sync.RWMutex
)

// GetCallSigns returns a thread-safe copy of the current callsigns
func GetCallSigns() []string {
	mu.RLock()
	defer mu.RUnlock()
	dst := make([]string, len(callSigns))
	copy(dst, callSigns)
	return dst
}

// SetCallSigns updates the global callsigns list in a thread-safe manner
func SetCallSigns(cs []string) {
	mu.Lock()
	defer mu.Unlock()
	callSigns = cs
}

// Unique returns a new slice with unique elements from the input slice.
func Unique(input []string) []string {
	if len(input) == 0 {
		return input
	}
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range input {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// ParseCallSigns parses a file and returns the list of callsigns
func ParseCallSigns(filePath string) ([]string, error) {
	var results []string
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()

		trimmedLine := strings.TrimSpace(line)

		// Exclude empty lines, or lines commented with # or //
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") || strings.HasPrefix(trimmedLine, "//") {
			continue
		}

		results = append(results, trimmedLine)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return results, nil
}

// FetchFromWeb fetches callsigns from a URL (expecting CSV format)
// It expects the data to be in the first column and skips the first row (header).
func FetchFromWeb(rawURL string) ([]string, error) {
	// Check if it's a Google Sheet edit URL and convert to export
	u, err := url.Parse(rawURL)
	if err == nil && strings.Contains(u.Host, "docs.google.com") && strings.Contains(u.Path, "/edit") {
		u.Path = strings.Replace(u.Path, "/edit", "/export", 1)
		q := u.Query()
		q.Set("format", "csv")
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}

	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	reader := csv.NewReader(resp.Body)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	var results []string
	// Skip first row (header)
	if len(records) > 1 {
		for _, row := range records[1:] {
			if len(row) > 0 {
				call := strings.TrimSpace(row[0])
				if call != "" {
					results = append(results, call)
				}
			}
		}
	}

	return results, nil
}
