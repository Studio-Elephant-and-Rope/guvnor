package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHello is a simple test to check the testing framework.
// In a real scenario, this would test functionality from main.go.
func TestHello(t *testing.T) {
	got := 1 + 1
	want := 2
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// TestMainOutput checks the output of the compiled main program.
// Since the main program now uses Cobra CLI, we check for expected CLI output.
func TestMainOutput(t *testing.T) {
	// Build the program first to ensure we are testing the current code
	cmdBuild := exec.Command("go", "build", "-o", "../../guvnor_test_binary") // Output to root to avoid dirtying cmd/guvnor
	cmdBuild.Dir = "."
	err := cmdBuild.Run()
	if err != nil {
		t.Fatalf("Failed to build main.go: %v", err)
	}
	defer os.Remove("../../guvnor_test_binary") // Clean up the binary

	// Test help output (default when no args provided)
	cmdRun := exec.Command("../../guvnor_test_binary")
	output, err := cmdRun.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run compiled main: %v, output: %s", err, string(output))
	}

	actualOutput := strings.TrimSpace(string(output))

	// Check for expected CLI output
	expectedStrings := []string{
		"Guvnor is a reliable, self-hostable incident management platform",
		"Usage:",
		"Available Commands:",
		"version",
		"run",
		"Flags:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(actualOutput, expected) {
			t.Errorf("Expected output to contain '%s', but it was missing from: %s", expected, actualOutput)
		}
	}
}

// TestVersionOutput tests the version command output.
func TestVersionOutput(t *testing.T) {
	// Build the program first
	cmdBuild := exec.Command("go", "build", "-o", "../../guvnor_test_binary")
	cmdBuild.Dir = "."
	err := cmdBuild.Run()
	if err != nil {
		t.Fatalf("Failed to build main.go: %v", err)
	}
	defer os.Remove("../../guvnor_test_binary")

	// Test version command
	cmdRun := exec.Command("../../guvnor_test_binary", "version")
	output, err := cmdRun.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run version command: %v, output: %s", err, string(output))
	}

	actualOutput := strings.TrimSpace(string(output))

	// Check for expected version output
	expectedStrings := []string{
		"Guvnor Incident Management Platform",
		"Version:",
		"Commit:",
		"Built:",
		"Go version:",
		"Go OS/Arch:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(actualOutput, expected) {
			t.Errorf("Expected version output to contain '%s', but it was missing from: %s", expected, actualOutput)
		}
	}
}
