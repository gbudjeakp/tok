package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/gbudjeakp/tok/internal/filter"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "check <command> [args...]",
		Short:              "Show which filter would handle a command (without running it)",
		DisableFlagParsing: true,
		Long: `tok check shows whether a command has a tok filter and which one would fire.
Useful for confirming that runner prefixes (uv run, npx, ...) resolve correctly.

Examples:
  tok check git log -20
  tok check uv run pytest -v
  tok check /usr/bin/docker ps`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// --help
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			return runCheck(args[0], args[1:])
		},
	}
}

func runCheck(cmd string, args []string) error {
	effectiveCmd, matched := filter.Filterable(cmd, args)

	input := cmd
	if len(args) > 0 {
		input += " " + strings.Join(args, " ")
	}

	if !matched {
		fmt.Printf("no filter  %s\n", input)
		if effectiveCmd != cmd {
			fmt.Printf("           (resolved: %s)\n", effectiveCmd)
		}
		return nil
	}

	fmt.Printf("filtered   %s\n", input)
	if effectiveCmd != cmd {
		fmt.Printf("           (resolved → %s)\n", effectiveCmd)
	}
	return nil
}
