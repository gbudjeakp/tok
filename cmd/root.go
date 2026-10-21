package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/gbudjeakp/tok/cmd.Version=...".
// GoReleaser fills this in from the git tag; `go install .` leaves it as "dev".
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "tok",
	Short:   "Three-layer AI token reducer",
	Version: Version,
	Long: `tok cuts LLM token usage across three layers:

  Layer 1  Shell output    tok git status, tok go test ./...
  Layer 2  Skill injection tok init injects brevity + cache hints into CLAUDE.md
  Layer 3  Analytics       tok gain shows unified savings dashboard

Install once, save forever:

  tok init -g          # Claude Code hook + CLAUDE.md skill
  tok init -g --codex  # Codex
  tok gain             # see what you saved`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newGainCmd())
	rootCmd.AddCommand(newHookCmd())
	rootCmd.AddCommand(newDiscoverCmd())
	rootCmd.AddCommand(newSessionCmd())
	rootCmd.AddCommand(newProveCmd())
	rootCmd.AddCommand(newLearnCmd())
	rootCmd.AddCommand(newCheckCmd())
	rootCmd.AddCommand(newWasteCmd())
	rootCmd.AddCommand(newLockdepsCmd())
	rootCmd.AddCommand(newBenchCmd())
	rootCmd.AddCommand(newModeCmd())
	rootCmd.AddCommand(newBoardCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(newVersionCmd())
}
