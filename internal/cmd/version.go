package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// Build information variables that are set during build time via -ldflags.
// These will be empty if not set during build, and we handle that gracefully.
var (
	// Version is the current version of Guvnor, typically from git tags.
	Version = "dev"

	// Commit is the git commit hash of the build.
	Commit = "unknown"

	// BuildDate is the date when the binary was built.
	BuildDate = "unknown"
)

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long: `Display detailed version information for Guvnor including:
  • Application version
  • Git commit hash
  • Build date
  • Go runtime version

This information is useful for debugging and support purposes.`,
	Run: func(cmd *cobra.Command, args []string) {
		displayVersionToWriter(cmd.OutOrStdout())
	},
}

// displayVersion prints the version information in a formatted manner to stdout.
func displayVersion() {
	displayVersionToWriter(os.Stdout)
}

// displayVersionToWriter prints the version information to the specified writer.
func displayVersionToWriter(w io.Writer) {
	fmt.Fprintf(w, "Guvnor Incident Management Platform\n")
	fmt.Fprintf(w, "Version:    %s\n", getVersionString())
	fmt.Fprintf(w, "Commit:     %s\n", getCommitString())
	fmt.Fprintf(w, "Built:      %s\n", getBuildDateString())
	fmt.Fprintf(w, "Go version: %s\n", runtime.Version())
	fmt.Fprintf(w, "Go OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// getVersionString returns the version string with appropriate fallback.
func getVersionString() string {
	if Version == "" || Version == "dev" {
		return "development"
	}
	return Version
}

// getCommitString returns the commit hash with appropriate fallback.
func getCommitString() string {
	if Commit == "" || Commit == "unknown" {
		return "unknown (development build)"
	}
	return Commit
}

// getBuildDateString returns the build date with appropriate fallback.
func getBuildDateString() string {
	if BuildDate == "" || BuildDate == "unknown" {
		return "unknown (development build)"
	}
	return BuildDate
}

// GetVersionInfo returns version information as a struct for programmatic access.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetVersionInfo returns structured version information for testing or API use.
func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   getVersionString(),
		Commit:    getCommitString(),
		BuildDate: getBuildDateString(),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// init registers the version command with the root command.
func init() {
	rootCmd.AddCommand(versionCmd)
}
