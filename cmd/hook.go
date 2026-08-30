package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/gbudjeakp/tok/internal/filter"
)

// toolInput is the JSON shape Claude Code sends to the hook on stdin.
type toolInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type bashInput struct {
	Command string `json:"command"`
}

// hookOutput is what we write to stdout to rewrite the command.
type hookOutput struct {
	ToolInput *bashInput `json:"tool_input,omitempty"`
}

func newHookCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "hook [agent]",
		Short:  "Internal: used by agent hooks to rewrite commands",
		Hidden: true, // not shown in help
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := "claude"
			if len(args) > 0 {
				agent = args[0]
			}
			switch agent {
			case "claude":
				return runClaudeHook()
			case "copilot":
				return runCopilotHook()
			default:
				return fmt.Errorf("unsupported agent: %s", agent)
			}
		},
	}
	return c
}

func runClaudeHook() error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return nil // nothing to rewrite
	}

	var input toolInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil // can't parse — let Claude handle it
	}

	if !strings.EqualFold(input.ToolName, "Bash") {
		return nil // only intercept Bash tool calls
	}

	var bash bashInput
	if err := json.Unmarshal(input.ToolInput, &bash); err != nil {
		return nil
	}

	rewritten := filter.Rewrite(bash.Command)
	if rewritten == "" {
		return nil // no filter for this command — passthrough
	}

	// Output modified tool_input so Claude Code executes the rewritten command
	out := hookOutput{ToolInput: &bashInput{Command: rewritten}}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// copilotHookInput is the JSON shape VS Code Copilot sends on stdin for PreToolUse.
type copilotHookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// runInTerminalInput mirrors the run_in_terminal tool's input fields.
type runInTerminalInput struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation,omitempty"`
	Goal        string `json:"goal,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

type copilotHookSpecificOutput struct {
	UpdatedInput       *runInTerminalInput `json:"updatedInput,omitempty"`
	PermissionDecision string              `json:"permissionDecision,omitempty"`
	Message            string              `json:"message,omitempty"`
}

type copilotHookOutput struct {
	HookSpecificOutput *copilotHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

func runCopilotHook() error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return nil
	}

	var input copilotHookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	switch input.ToolName {
	case "run_in_terminal":
		return handleCopilotTerminal(input.ToolInput)
	case "read_file":
		return handleCopilotReadFile(input.ToolInput)
	default:
		return nil
	}
}

func handleCopilotTerminal(rawInput json.RawMessage) error {
	var term runInTerminalInput
	if err := json.Unmarshal(rawInput, &term); err != nil {
		return nil
	}

	rewritten := filter.Rewrite(term.Command)
	if rewritten == "" {
		return nil // no filter — passthrough
	}

	updated := term
	updated.Command = rewritten
	out := copilotHookOutput{
		HookSpecificOutput: &copilotHookSpecificOutput{
			UpdatedInput:       &updated,
			PermissionDecision: "allow",
		},
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// readFileInput mirrors the read_file tool's input fields.
type readFileInput struct {
	FilePath  string `json:"filePath,omitempty"`
	File      string `json:"file,omitempty"` // some versions use "file"
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
}

func handleCopilotReadFile(rawInput json.RawMessage) error {
	var rf readFileInput
	if err := json.Unmarshal(rawInput, &rf); err != nil {
		return nil
	}
	path := rf.FilePath
	if path == "" {
		path = rf.File
	}
	if path == "" {
		return nil
	}

	if reason := guardRead(path, rf.StartLine, rf.EndLine); reason != "" {
		out := copilotHookOutput{
			HookSpecificOutput: &copilotHookSpecificOutput{
				PermissionDecision: "deny",
				Message:            reason,
			},
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	return nil
}

// guardRead returns a deny reason if the file path should be blocked, or "".
// A non-zero endLine (bounded read) is always allowed.
func guardRead(path string, startLine, endLine int) string {
	// Bounded reads (offset + limit) are always allowed.
	if endLine > 0 && endLine-startLine <= 400 {
		return ""
	}

	base := filepath.Base(path)
	lowerBase := strings.ToLower(base)
	lowerPath := strings.ToLower(path)

	// Lock files — reading them rarely helps; use `tok lockdeps` instead.
	lockFiles := map[string]bool{
		"package-lock.json": true, "yarn.lock": true, "bun.lockb": true,
		"go.sum": true, "cargo.lock": true, "poetry.lock": true,
		"composer.lock": true, "gemfile.lock": true, "pnpm-lock.yaml": true,
		"uv.lock": true, "pipfile.lock": true,
	}
	if lockFiles[lowerBase] {
		return fmt.Sprintf("[tok guard] Blocked reading %s (lock file). "+
			"Run `tok lockdeps %s` for a compact dependency table instead.", base, path)
	}

	// Minified / generated files.
	if strings.HasSuffix(lowerBase, ".min.js") || strings.HasSuffix(lowerBase, ".min.css") ||
		strings.HasSuffix(lowerBase, ".map") || strings.HasSuffix(lowerBase, ".bundle.js") {
		return fmt.Sprintf("[tok guard] Blocked reading %s (minified/generated file). "+
			"Inspect the source file instead.", base)
	}

	// Noise directories.
	noiseDirs := []string{"/node_modules/", "/dist/", "/build/", "/.next/", "/vendor/", "/coverage/"}
	for _, d := range noiseDirs {
		if strings.Contains(lowerPath, d) {
			return fmt.Sprintf("[tok guard] Blocked reading %s (inside generated directory %s). "+
				"Read the source file directly.", base, strings.Trim(d, "/"))
		}
	}

	// Large file check (only if we can stat without error).
	info, err := os.Stat(path)
	if err == nil && info.Size() > 512*1024 { // 512 KB
		return fmt.Sprintf("[tok guard] Blocked reading %s (%dKB). "+
			"Use a bounded read (startLine/endLine) or `tok <cmd>` to query it.", base, info.Size()/1024)
	}

	return ""
}
