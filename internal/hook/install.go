// Package hook installs the Claude Code PreToolUse hook that auto-rewrites commands.
package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ClaudeHookEntry is the minimal settings.json shape we need to merge.
type claudeSettings struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

type hookGroup struct {
	Matcher string     `json:"matcher"`
	Hooks   []hookDef  `json:"hooks"`
}

type hookDef struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// InstallClaude installs the PreToolUse hook into ~/.claude/settings.json.
func InstallClaude(tokPath string) error {
	settingsPath, err := claudeSettingsPath()
	if err != nil {
		return err
	}

	settings, err := loadOrCreate(settingsPath)
	if err != nil {
		return err
	}

	// Inject the PreToolUse hook if not already present
	cmd := tokPath + " hook claude"
	if alreadyInstalled(settings, "PreToolUse", cmd) {
		fmt.Println("Hook already installed in", settingsPath)
		return nil
	}

	if settings.Hooks == nil {
		settings.Hooks = map[string][]hookGroup{}
	}
	settings.Hooks["PreToolUse"] = append(
		removeTokHook(settings.Hooks["PreToolUse"], "PreToolUse"),
		hookGroup{
			Matcher: "Bash",
			Hooks:   []hookDef{{Type: "command", Command: cmd}},
		},
	)

	if err := save(settingsPath, settings); err != nil {
		return err
	}
	fmt.Println("✓ Hook installed:", settingsPath)
	return nil
}

// UninstallClaude removes the tok hook from ~/.claude/settings.json.
func UninstallClaude() error {
	settingsPath, err := claudeSettingsPath()
	if err != nil {
		return err
	}
	settings, err := loadOrCreate(settingsPath)
	if err != nil {
		return err
	}
	settings.Hooks["PreToolUse"] = removeTokHook(settings.Hooks["PreToolUse"], "tok hook claude")
	return save(settingsPath, settings)
}

func claudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func loadOrCreate(path string) (*claudeSettings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &claudeSettings{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		// File exists but isn't valid JSON — back it up and start fresh
		_ = os.Rename(path, path+".bak")
		return &claudeSettings{}, nil
	}
	return &s, nil
}

func save(path string, s *claudeSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func alreadyInstalled(s *claudeSettings, event, cmd string) bool {
	for _, g := range s.Hooks[event] {
		for _, h := range g.Hooks {
			if h.Command == cmd {
				return true
			}
		}
	}
	return false
}

func removeTokHook(groups []hookGroup, _ string) []hookGroup {
	var out []hookGroup
	for _, g := range groups {
		var hooks []hookDef
		for _, h := range g.Hooks {
			if !strings.Contains(h.Command, "tok hook") {
				hooks = append(hooks, h)
			}
		}
		if len(hooks) > 0 {
			g.Hooks = hooks
			out = append(out, g)
		}
	}
	return out
}

// TokBinaryPath returns the absolute path to the tok binary.
func TokBinaryPath() (string, error) {
	p, err := exec.LookPath("tok")
	if err != nil {
		// Fall back to the binary running right now
		return os.Executable()
	}
	return p, nil
}

// copilotHookFile is the JSON written to ~/.copilot/hooks/tok.json.
type copilotHookFile struct {
	Hooks map[string][]copilotHookDef `json:"hooks"`
}

type copilotHookDef struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// InstallCopilot writes a PreToolUse hook file to ~/.copilot/hooks/tok.json.
func InstallCopilot(tokPath string) error {
	hookPath, err := copilotHookPath()
	if err != nil {
		return err
	}

	cmd := tokPath + " hook copilot"
	hf := copilotHookFile{
		Hooks: map[string][]copilotHookDef{
			"PreToolUse": {{Type: "command", Command: cmd, Timeout: 10}},
		},
	}
	data, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(hookPath, data, 0600); err != nil {
		return err
	}
	fmt.Println("✓ Copilot hook installed:", hookPath)
	return nil
}

// UninstallCopilot removes ~/.copilot/hooks/tok.json.
func UninstallCopilot() error {
	hookPath, err := copilotHookPath()
	if err != nil {
		return err
	}
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("✓ Copilot hook removed")
	return nil
}

func copilotHookPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".copilot", "hooks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "tok.json"), nil
}
