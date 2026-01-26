package hams

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseCallSigns(t *testing.T) {
	content := `
# Comment
K6POTA
W6SOTA
// Another comment

N6HAM
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "calls.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	calls, err := ParseCallSigns(tmpFile)
	if err != nil {
		t.Fatalf("ParseCallSigns failed: %v", err)
	}

	expected := []string{"K6POTA", "W6SOTA", "N6HAM"}
	if len(calls) != len(expected) {
		t.Errorf("Expected %d calls, got %d", len(expected), len(calls))
	}

	for _, exp := range expected {
		if !slices.Contains(calls, exp) {
			t.Errorf("Expected %s not found", exp)
		}
	}
}

func TestFetchFromWeb(t *testing.T) {
	// Mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Header1,Header2") // Header
		fmt.Fprintln(w, "K6POTA,Test Park")
		fmt.Fprintln(w, "W6SOTA,Test Peak")
		fmt.Fprintln(w, "")            // Empty line
		fmt.Fprintln(w, ",Empty Call") // Empty call
	}))
	defer ts.Close()

	calls, err := FetchFromWeb(ts.URL)
	if err != nil {
		t.Fatalf("FetchFromWeb failed: %v", err)
	}

	expected := []string{"K6POTA", "W6SOTA"}
	if len(calls) != len(expected) {
		t.Errorf("Expected %d calls, got %d", len(expected), len(calls))
	}

	if !slices.Contains(calls, "K6POTA") {
		t.Error("Missing K6POTA")
	}
	if !slices.Contains(calls, "W6SOTA") {
		t.Error("Missing W6SOTA")
	}
}

func TestSetAndGetCallSigns(t *testing.T) {
	initial := []string{"A", "B"}
	SetCallSigns(initial)

	got := GetCallSigns()
	if !slices.Equal(initial, got) {
		t.Errorf("GetCallSigns mismatch. Want %v, got %v", initial, got)
	}

	// Test that modifying the result of GetCallSigns doesn't affect internal state
	got[0] = "C"
	got2 := GetCallSigns()
	if got2[0] != "A" {
		t.Error("Internal state modified by returned slice mutation")
	}
}

func TestUnique(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no duplicates",
			input:    []string{"A", "B", "C"},
			expected: []string{"A", "B", "C"},
		},
		{
			name:     "with duplicates",
			input:    []string{"A", "B", "A", "C", "B"},
			expected: []string{"A", "B", "C"},
		},
		{
			name:     "empty",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unique(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("Expected length %d, got %d", len(tt.expected), len(got))
			}
			for _, v := range tt.expected {
				if !slices.Contains(got, v) {
					t.Errorf("Expected %s not found in result %v", v, got)
				}
			}
		})
	}
}
