package hams

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Define as a public variable
var CallSigns []string

// This function parses the
func ParseCallSigns(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		// Return the error so the calling code can exit or handle it.
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
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

		// Store callsigns
		CallSigns = append(CallSigns, trimmedLine)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	return nil
}
