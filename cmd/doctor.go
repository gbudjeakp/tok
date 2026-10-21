package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/gbudjeakp/tok/internal/config"
	"github.com/gbudjeakp/tok/internal/skill"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose tok install health — binary, hooks, skill, PATH",
		RunE: func(cmd *cobra.Command, args []string) error {
			runDoctor()
			return nil
		},
	}
}

func runDoctor() {
	ok := func(cond bool) string {
		if cond {
			return "✓"
		}
		return "✗"
	}

	fmt.Printf("tok doctor — version %s\n\n", Version)

	// Binary on PATH
	binPath, err := exec.LookPath("tok")
	onPath := err == nil
	fmt.Printf("%s  tok on PATH: %s\n", ok(onPath), orNone(binPath))

	home, _ := os.UserHomeDir()

	// Copilot hook
	copilotHook := filepath.Join(home, ".copilot", "hooks", "tok.json")
	copilotInstalled := fileContains(copilotHook, "hook copilot")
	fmt.Printf("%s  Copilot hook: %s\n", ok(copilotInstalled), copilotHook)

	// Claude hook
	claudeSettings := filepath.Join(home, ".claude", "settings.json")
	claudeInstalled := fileContains(claudeSettings, "hook claude")
	fmt.Printf("%s  Claude Code hook: %s\n", ok(claudeInstalled), claudeSettings)

	// Skill injection
	files := skill.InjectedFiles()
	fmt.Printf("%s  Skill injected: %d file(s)\n", ok(len(files) > 0), len(files))
	for _, f := range files {
		fmt.Printf("     %s\n", f)
	}

	// Mode
	mode := config.Load().Mode
	fmt.Printf("✓  Skill mode: %s\n", mode)

	// rtk conflict check
	rtkHook := filepath.Join(home, ".copilot", "hooks", "rtk-rewrite.json")
	if _, err := os.Stat(rtkHook); err == nil {
		fmt.Printf("✗  rtk hook also present (%s) — remove it, running both double-executes commands\n", rtkHook)
	}

	fmt.Println()
	if onPath && (copilotInstalled || claudeInstalled) && len(files) > 0 {
		fmt.Println("All layers active.")
	} else {
		fmt.Println("Some layers missing. Run 'tok init -g [--copilot]' to fix.")
	}
}

func orNone(s string) string {
	if s == "" {
		return "(not found)"
	}
	return s
}

func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}
