package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tok/internal/analytics"
)

func newDiscoverCmd() *cobra.Command {
	var since int

	c := &cobra.Command{
		Use:   "discover",
		Short: "Find commands in shell history that tok could optimize",
		Long: `tok discover scans your shell history and reports commands tok can filter.
Use it to find missed savings opportunities.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscover(since)
		},
	}
	c.Flags().IntVar(&since, "since", 7, "look back N days in shell history")
	return c
}

func runDiscover(sinceDays int) error {
	hist := shellHistory()
	if len(hist) == 0 {
		fmt.Println("No shell history found. Run some commands first.")
		return nil
	}

	// Load existing analytics to skip already-filtered commands
	recorded, _ := analytics.Load()
	alreadyFiltered := map[string]bool{}
	for _, e := range recorded {
		alreadyFiltered[e.Cmd] = true
	}

	// Count opportunities
	type opp struct {
		cmd   string
		count int
		est   string
	}
	counts := map[string]int{}
	for _, line := range hist {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		rewritten := rewriteCommand(parts[0], parts[1:])
		if rewritten != "" && !alreadyFiltered[parts[0]] {
			counts[parts[0]]++
		}
	}

	if len(counts) == 0 {
		fmt.Println("Great — tok already covers your most common commands.")
		fmt.Println("Run 'tok gain' to see savings so far.")
		return nil
	}

	fmt.Println("Commands tok can filter (not yet using tok):")
	fmt.Println()
	for cmd, n := range counts {
		est := estimateSavings(cmd)
		fmt.Printf("  %-20s  %3d uses  ~%s savings\n", cmd, n, est)
	}
	fmt.Println()
	fmt.Println("Run 'tok init -g' to install the auto-rewrite hook.")
	return nil
}

// rewriteCommand returns a non-empty string if tok has a filter for this command.
func rewriteCommand(cmd string, args []string) string {
	supported := map[string]bool{
		"git": true, "go": true, "cargo": true, "npm": true, "yarn": true,
		"pnpm": true, "bun": true, "pytest": true, "python": true,
		"docker": true, "kubectl": true, "ls": true, "find": true,
		"grep": true, "rg": true, "ruff": true, "eslint": true,
		"tsc": true, "golangci-lint": true,
	}
	if supported[cmd] {
		return "tok " + cmd
	}
	return ""
}

func estimateSavings(cmd string) string {
	estimates := map[string]string{
		"git":         "60–80%",
		"go":          "85–95%",
		"cargo":       "85–95%",
		"npm":         "70–90%",
		"pytest":      "80–90%",
		"docker":      "50–70%",
		"kubectl":     "50–70%",
		"grep":        "40–70%",
		"ls":          "30–60%",
		"ruff":        "50–70%",
		"eslint":      "50–70%",
	}
	if s, ok := estimates[cmd]; ok {
		return s
	}
	return "30–60%"
}

func shellHistory() []string {
	home, _ := os.UserHomeDir()
	histFiles := []string{
		home + "/.zsh_history",
		home + "/.bash_history",
		home + "/.local/share/fish/fish_history",
	}

	cutoff := time.Now().AddDate(0, 0, -7).Unix()
	var lines []string

	for _, f := range histFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			// zsh history format: ": timestamp:0;command"
			if strings.HasPrefix(l, ": ") {
				parts := strings.SplitN(l, ";", 2)
				if len(parts) == 2 {
					// Check timestamp
					var ts int64
					fmt.Sscanf(parts[0], ": %d:", &ts)
					if ts > 0 && ts < cutoff {
						continue
					}
					l = parts[1]
				}
			}
			if l != "" && !strings.HasPrefix(l, "#") {
				lines = append(lines, l)
			}
		}
	}

	// Also check recent git log of shell commands via `fc -l` if available
	out, err := exec.Command("bash", "-c", "history 500 2>/dev/null || true").Output()
	if err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			// bash history format: "  123  command"
			parts := strings.SplitN(strings.TrimSpace(l), " ", 2)
			if len(parts) == 2 {
				lines = append(lines, strings.TrimSpace(parts[1]))
			}
		}
	}

	return lines
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("tok v0.1.0")
		},
	}
}
