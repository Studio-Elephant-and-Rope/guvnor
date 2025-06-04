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

	expectedOutput := "Hello, Guvnor!"
	actualOutput := strings.TrimSpace(string(output))

	if actualOutput != expectedOutput {
		t.Errorf("Expected output '%s', but got '%s'", expectedOutput, actualOutput)
	}
}
