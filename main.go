package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gbudjeakp/tok/cmd"
	"github.com/gbudjeakp/tok/internal/filter"
)

// knownSubcmds are cobra-handled; everything else routes to the filter engine.
var knownSubcmds = map[string]bool{
	"init": true, "gain": true, "hook": true, "discover": true,
	"session": true, "prove": true, "learn": true, "check": true,
	"waste": true, "lockdeps": true, "bench": true, "mode": true, "board": true,
	"version": true, "help": true, "completion": true,
}

func main() {
	maybeAutoLearn()
	if len(os.Args) > 1 {
		first := os.Args[1]
		if !knownSubcmds[first] && !strings.HasPrefix(first, "-") {
			os.Exit(filter.RunCommand(os.Args[1], os.Args[2:]))
		}
	}
	cmd.Execute()
}

// maybeAutoLearn spawns tok learn --global in the background if the last run
// was more than 24h ago. Completely silent — never blocks the caller.
func maybeAutoLearn() {
	// Skip during hook invocations (called with "hook" subcommand) to avoid loops.
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	stampFile := filepath.Join(home, ".tok", "last-learn")
	if data, err := os.ReadFile(stampFile); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			if time.Since(t) < 24*time.Hour {
				return
			}
		}
	}
	// Write timestamp before spawning so concurrent invocations don't double-fire.
	_ = os.MkdirAll(filepath.Dir(stampFile), 0700)
	_ = os.WriteFile(stampFile, []byte(time.Now().Format(time.RFC3339)), 0600)
	exe, err := os.Executable()
	if err != nil {
		return
	}
	proc := exec.Command(exe, "learn", "--global")
	_ = proc.Start()
	// Intentionally not calling proc.Wait() — fire and forget.
}
