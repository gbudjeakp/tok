package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/gbudjeakp/tok/internal/config"
	"github.com/gbudjeakp/tok/internal/skill"
)

func newModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mode [lite|full|ultra|off]",
		Short: "Set or show the skill injection intensity",
		Long: `tok mode controls how aggressive the injected brevity skill is:

  lite   short brevity rule only
  full   brevity + cache-alignment hints (default)
  ultra  full + "minimal diff" rule (reuse/stdlib before writing new code)
  off    removes the skill block entirely

With no argument, prints the current mode. Re-injects into every instruction
file that already has a tok skill block.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println(config.Load().Mode)
				return nil
			}
			mode := args[0]
			switch mode {
			case "lite", "full", "ultra", "off":
			default:
				return fmt.Errorf("invalid mode %q — use lite, full, ultra, or off", mode)
			}

			if err := config.Save(config.Config{Mode: mode}); err != nil {
				return err
			}

			files := skill.InjectedFiles()
			if len(files) == 0 {
				fmt.Printf("Mode set to %s. Run 'tok init' to inject the skill.\n", mode)
				return nil
			}
			for _, f := range files {
				if err := skill.Inject(f, mode); err != nil {
					fmt.Printf("skill %s: %v\n", f, err)
					continue
				}
				fmt.Printf("✓ %s → %s\n", mode, f)
			}
			return nil
		},
	}
}
