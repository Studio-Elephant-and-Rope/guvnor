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
// Since the main program now uses structured logging, we check for expected log entries.
func TestMainOutput(t *testing.T) {
	// Build the program first to ensure we are testing the current code
	cmdBuild := exec.Command("go", "build", "-o", "../../guvnor_test_binary") // Output to root to avoid dirtying cmd/guvnor
	cmdBuild.Dir = "."
	err := cmdBuild.Run()
	if err != nil {
		t.Fatalf("Failed to build main.go: %v", err)
	}
	defer os.Remove("../../guvnor_test_binary") // Clean up the binary

	cmdRun := exec.Command("../../guvnor_test_binary")
	output, err := cmdRun.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run compiled main: %v, output: %s", err, string(output))
	}

	actualOutput := strings.TrimSpace(string(output))

	// Check for expected structured logging output
	expectedStrings := []string{
		"Starting Guvnor incident management platform",
		"version=0.1.0",
		"environment=development",
		"Application initialized successfully",
		"Operation started",
		"Operation completed",
		"Guvnor startup complete",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(actualOutput, expected) {
			t.Errorf("Expected output to contain '%s', but it was missing from: %s", expected, actualOutput)
		}
	}

	// Verify structured logging format (should contain time, level, source, msg)
	if !strings.Contains(actualOutput, "time=") {
		t.Error("Expected structured logging output to contain timestamp")
	}
	if !strings.Contains(actualOutput, "level=") {
		t.Error("Expected structured logging output to contain log level")
	}
	if !strings.Contains(actualOutput, "source=") {
		t.Error("Expected structured logging output to contain source information")
	}
	if !strings.Contains(actualOutput, "msg=") {
		t.Error("Expected structured logging output to contain message field")
	}
}
