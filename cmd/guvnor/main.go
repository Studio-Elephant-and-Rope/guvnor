package main

import (
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/cmd"
)

// main is the entry point for the Guvnor CLI application.
//
// This function initialises the Cobra CLI framework and executes
// the appropriate command based on user input.
func main() {
	// Execute the root command and all subcommands
	cmd.Execute()
}
