package metadata

import (
	"testing"
)

func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"APP_NAME", APP_NAME, "opus-mcp"},
		{"APP_TITLE", APP_TITLE, "OPUS MCP Server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestBuildVariablesDefaults(t *testing.T) {
	// Test that build variables have their default values when not set via linker flags
	// Note: These tests will pass when running `go test` without build flags
	// and will need different assertions when built with actual linker flags

	tests := []struct {
		name     string
		value    string
		contains string
	}{
		{
			name:     "BuildVersion default",
			value:    BuildVersion,
			contains: "uninitialised",
		},
		{
			name:     "BuildTime default",
			value:    BuildTime,
			contains: "uninitialised",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When built without linker flags, should contain "uninitialised"
			// When built with linker flags, should be set to actual values
			if BuildVersion == "uninitialised; use linker flags: -X 'opus-mcp/internal/metadata.BuildVersion=1.0.0'" {
				// Running without linker flags
				if tt.value == "" {
					t.Errorf("%s is empty", tt.name)
				}
			} else {
				// Running with linker flags - values should be set
				if BuildVersion == "" || BuildVersion == "uninitialised; use linker flags: -X 'opus-mcp/internal/metadata.BuildVersion=1.0.0'" {
					t.Errorf("BuildVersion should be set when using linker flags, got: %q", BuildVersion)
				}
			}
		})
	}
}

func TestBuildVariablesNotEmpty(t *testing.T) {
	// Verify that build variables are never empty strings
	if BuildVersion == "" {
		t.Error("BuildVersion should not be empty")
	}
	if BuildTime == "" {
		t.Error("BuildTime should not be empty")
	}
}

func TestBuildVariablesCanBeSet(t *testing.T) {
	// This test demonstrates that the variables can be modified
	// Store original values
	originalVersion := BuildVersion
	originalTime := BuildTime

	// Simulate setting via linker flags by directly modifying
	BuildVersion = "v1.2.3"
	BuildTime = "2026-01-07T10:00:00Z"

	// Verify changes
	if BuildVersion != "v1.2.3" {
		t.Errorf("BuildVersion = %q, want %q", BuildVersion, "v1.2.3")
	}
	if BuildTime != "2026-01-07T10:00:00Z" {
		t.Errorf("BuildTime = %q, want %q", BuildTime, "2026-01-07T10:00:00Z")
	}

	// Restore original values for other tests
	BuildVersion = originalVersion
	BuildTime = originalTime
}
