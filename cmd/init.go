package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tok/internal/hook"
	"tok/internal/skill"
)

func newInitCmd() *cobra.Command {
	var global bool
	var copilot bool
	var uninstall bool
	var noSkill bool

	c := &cobra.Command{
		Use:   "init",
		Short: "Install hook + inject tok skill into agent instruction files",
		Long: `tok init sets up all three layers:

  1. Injects the brevity + cache-alignment skill into your agent's instruction
     file. This reduces output tokens and keeps provider KV-cache hits stable.
  2. Installs a PreToolUse hook so your agent automatically rewrites commands
     to tok equivalents before execution.

Examples:
  tok init -g              # Claude Code (hook + CLAUDE.md skill)
  tok init -g --copilot    # VS Code Copilot (~/.copilot/copilot-instructions.md)
  tok init --uninstall     # remove everything`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if uninstall {
				return runUninstall(copilot)
			}

			fmt.Println("Installing tok…")

			// Layer 1: hook (Claude Code only for now; Copilot hook via instructions)
			if !copilot {
				tokPath, err := hook.TokBinaryPath()
				if err != nil {
					tokPath, _ = os.Executable()
				}
				if err := hook.InstallClaude(tokPath); err != nil {
					fmt.Fprintln(os.Stderr, "hook:", err)
				}
			} else {
				tokPath, err := hook.TokBinaryPath()
				if err != nil {
					tokPath, _ = os.Executable()
				}
				if err := hook.InstallCopilot(tokPath); err != nil {
					fmt.Fprintln(os.Stderr, "hook:", err)
				}
			}

			// Layer 2: skill injection
			if !noSkill {
				injected := 0
				for _, path := range skill.DefaultFiles(copilot) {
					if err := skill.Inject(path); err != nil {
						fmt.Fprintf(os.Stderr, "skill %s: %v\n", path, err)
						continue
					}
					fmt.Printf("✓ Skill injected: %s\n", path)
					injected++
				}
				if injected == 0 {
					fmt.Println("  (no instruction file found — will be created)")
				}
			}

			if copilot {
				fmt.Println("\nDone. Reload VS Code window for the skill to take effect.")
			} else {
				fmt.Println("\nDone. Restart Claude Code for the hook to take effect.")
			}
			fmt.Println("Run 'tok gain' after a few sessions to see your savings.")
			return nil
		},
	}

	c.Flags().BoolVarP(&global, "global", "g", false, "global install (recommended)")
	c.Flags().BoolVar(&copilot, "copilot", false, "VS Code Copilot mode (injects into ~/.copilot/copilot-instructions.md)")
	c.Flags().BoolVar(&uninstall, "uninstall", false, "remove hook and skill")
	c.Flags().BoolVar(&noSkill, "no-skill", false, "skip skill injection")
	return c
}

func runUninstall(copilot bool) error {
	if !copilot {
		if err := hook.UninstallClaude(); err != nil {
			fmt.Fprintln(os.Stderr, "hook:", err)
		} else {
			fmt.Println("✓ Hook removed")
		}
	} else {
		if err := hook.UninstallCopilot(); err != nil {
			fmt.Fprintln(os.Stderr, "hook:", err)
		}
	}
	for _, path := range skill.DefaultFiles(copilot) {
		if err := skill.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "skill %s: %v\n", path, err)
		}
	}
	fmt.Println("✓ Skill blocks removed")
	return nil
}
