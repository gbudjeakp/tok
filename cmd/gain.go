package cmd

import (
	"github.com/spf13/cobra"
	"tok/internal/analytics"
)

func newGainCmd() *cobra.Command {
	var history bool

	c := &cobra.Command{
		Use:   "gain",
		Short: "Show token savings dashboard",
		Long: `tok gain shows a unified savings dashboard across all three layers:

  - Shell output tokens saved (measured, Layer 1)
  - Estimated input tokens saved by cache alignment (Layer 2)
  - Output tokens saved by brevity skill (Layer 2, estimated)

Flags:
  --history    Show recent command history`,
		RunE: func(cmd *cobra.Command, args []string) error {
			analytics.Show(history)
			return nil
		},
	}

	c.Flags().BoolVar(&history, "history", false, "show recent command history")
	return c
}
