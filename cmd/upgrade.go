package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade tok to the latest version",
		Long: `tok upgrade runs "go install github.com/gbudjeakp/tok@latest" and reports
the version change. Requires Go to be installed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade()
		},
	}
}

func runUpgrade() error {
	before := Version
	fmt.Printf("Current version: %s\n", before)

	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go not found on PATH — install Go or download a binary from https://github.com/gbudjeakp/tok/releases")
	}

	fmt.Println("Running: go install github.com/gbudjeakp/tok@latest")
	c := exec.Command("go", "install", "github.com/gbudjeakp/tok@latest")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("go install failed: %w", err)
	}

	binPath, err := exec.LookPath("tok")
	if err != nil {
		fmt.Println("Installed, but 'tok' is not on PATH yet — add $(go env GOPATH)/bin to PATH.")
		return nil
	}

	out, err := exec.Command(binPath, "version").Output()
	if err != nil {
		fmt.Println("Upgrade complete, but could not verify new version.")
		return nil
	}

	fmt.Printf("Now running: %s", out)
	return nil
}
