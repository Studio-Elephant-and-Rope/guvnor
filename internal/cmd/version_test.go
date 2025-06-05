package cmd

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionCmd(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectError    bool
		expectContains []string
	}{
		{
			name:        "version command",
			args:        []string{"version"},
			expectError: false,
			expectContains: []string{
				"Guvnor Incident Management Platform",
				"Version:",
				"Commit:",
				"Built:",
				"Go version:",
				"Go OS/Arch:",
			},
		},
		{
			name:        "version command with help",
			args:        []string{"version", "--help"},
			expectError: false,
			expectContains: []string{
				"Display detailed version information",
				"Application version",
				"Git commit hash",
				"Build date",
				"Go runtime version",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new command for each test
			cmd := GetRootCmd()

			// Capture output
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Set arguments
			cmd.SetArgs(tt.args)

			// Execute command
			err := cmd.Execute()

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check output contains expected strings
			output := buf.String()
			for _, expected := range tt.expectContains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain '%s', got: %s", expected, output)
				}
			}
		})
	}
}

func TestGetVersionString(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		expected     string
	}{
		{
			name:     "development version",
			version:  "dev",
			expected: "development",
		},
		{
			name:     "empty version",
			version:  "",
			expected: "development",
		},
		{
			name:     "tagged version",
			version:  "v1.0.0",
			expected: "v1.0.0",
		},
		{
			name:     "commit version",
			version:  "abcd123",
			expected: "abcd123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original version
			originalVersion := Version
			defer func() { Version = originalVersion }()

			// Set test version
			Version = tt.version

			// Test function
			result := getVersionString()
			if result != tt.expected {
				t.Errorf("getVersionString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetCommitString(t *testing.T) {
	tests := []struct {
		name     string
		commit   string
		expected string
	}{
		{
			name:     "unknown commit",
			commit:   "unknown",
			expected: "unknown (development build)",
		},
		{
			name:     "empty commit",
			commit:   "",
			expected: "unknown (development build)",
		},
		{
			name:     "valid commit",
			commit:   "abcd123",
			expected: "abcd123",
		},
		{
			name:     "full commit hash",
			commit:   "abcd1234567890abcdef1234567890abcdef1234",
			expected: "abcd1234567890abcdef1234567890abcdef1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original commit
			originalCommit := Commit
			defer func() { Commit = originalCommit }()

			// Set test commit
			Commit = tt.commit

			// Test function
			result := getCommitString()
			if result != tt.expected {
				t.Errorf("getCommitString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetBuildDateString(t *testing.T) {
	tests := []struct {
		name      string
		buildDate string
		expected  string
	}{
		{
			name:      "unknown build date",
			buildDate: "unknown",
			expected:  "unknown (development build)",
		},
		{
			name:      "empty build date",
			buildDate: "",
			expected:  "unknown (development build)",
		},
		{
			name:      "valid build date",
			buildDate: "2023-12-25_14:30:00",
			expected:  "2023-12-25_14:30:00",
		},
		{
			name:      "ISO format build date",
			buildDate: "2023-12-25T14:30:00Z",
			expected:  "2023-12-25T14:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original build date
			originalBuildDate := BuildDate
			defer func() { BuildDate = originalBuildDate }()

			// Set test build date
			BuildDate = tt.buildDate

			// Test function
			result := getBuildDateString()
			if result != tt.expected {
				t.Errorf("getBuildDateString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetVersionInfo(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	// Set test values
	Version = "v1.2.3"
	Commit = "abc123"
	BuildDate = "2023-12-25_14:30:00"

	info := GetVersionInfo()

	// Test version info structure
	if info.Version != "v1.2.3" {
		t.Errorf("Expected version v1.2.3, got %s", info.Version)
	}

	if info.Commit != "abc123" {
		t.Errorf("Expected commit abc123, got %s", info.Commit)
	}

	if info.BuildDate != "2023-12-25_14:30:00" {
		t.Errorf("Expected build date 2023-12-25_14:30:00, got %s", info.BuildDate)
	}

	if info.GoVersion != runtime.Version() {
		t.Errorf("Expected Go version %s, got %s", runtime.Version(), info.GoVersion)
	}

	if info.OS != runtime.GOOS {
		t.Errorf("Expected OS %s, got %s", runtime.GOOS, info.OS)
	}

	if info.Arch != runtime.GOARCH {
		t.Errorf("Expected arch %s, got %s", runtime.GOARCH, info.Arch)
	}
}

func TestGetVersionInfoWithDefaults(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	// Set default/unknown values
	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"

	info := GetVersionInfo()

	// Test that defaults are handled correctly
	if info.Version != "development" {
		t.Errorf("Expected version development, got %s", info.Version)
	}

	if !strings.Contains(info.Commit, "unknown (development build)") {
		t.Errorf("Expected commit to contain 'unknown (development build)', got %s", info.Commit)
	}

	if !strings.Contains(info.BuildDate, "unknown (development build)") {
		t.Errorf("Expected build date to contain 'unknown (development build)', got %s", info.BuildDate)
	}
}

func TestDisplayVersion(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	BuildDate = "2023-12-25_14:30:00"

	// We can't easily test displayVersion directly since it writes to stdout,
	// but we can test the individual components it uses
	versionStr := getVersionString()
	commitStr := getCommitString()
	buildDateStr := getBuildDateString()

	if versionStr != "v1.0.0" {
		t.Errorf("Expected version string v1.0.0, got %s", versionStr)
	}

	if commitStr != "abc123" {
		t.Errorf("Expected commit string abc123, got %s", commitStr)
	}

	if buildDateStr != "2023-12-25_14:30:00" {
		t.Errorf("Expected build date string 2023-12-25_14:30:00, got %s", buildDateStr)
	}

	// Test that all expected information is present in the output format
	expectedElements := []string{
		versionStr,
		commitStr,
		buildDateStr,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	}

	for _, element := range expectedElements {
		if element == "" {
			t.Error("Version display element should not be empty")
		}
	}
}

func TestVersionCmdExists(t *testing.T) {
	// Test that the version command is properly registered
	cmd := GetRootCmd()

	var versionCmd *cobra.Command
	for _, subCmd := range cmd.Commands() {
		if subCmd.Use == "version" {
			versionCmd = subCmd
			break
		}
	}

	if versionCmd == nil {
		t.Fatal("Version command should be registered with root command")
	}

	if versionCmd.Short == "" {
		t.Error("Version command should have a short description")
	}

	if versionCmd.Long == "" {
		t.Error("Version command should have a long description")
	}

	if versionCmd.Run == nil {
		t.Error("Version command should have a Run function")
	}
}
