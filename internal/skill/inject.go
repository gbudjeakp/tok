// Package skill injects tok's brevity and cache-alignment hints into agent instruction files.
package skill

import (
	"os"
	"path/filepath"
	"strings"
)

const blockStart = "<!-- tok-skill: start — remove this block to disable -->"
const blockEnd = "<!-- tok-skill: end -->"

const liteBlock = `
<!-- tok-skill: start — remove this block to disable -->
## Tok: Response Brevity
Be terse. Drop filler and hedging. Never drop not/never/no, numbers, exact errors.
<!-- tok-skill: end -->
`

const fullBlock = `
<!-- tok-skill: start — remove this block to disable -->
## Tok: Response Brevity
Be terse. Drop: articles, pleasantries, hedging, filler.
Never drop: not/never/no, numbers, technical terms, exact errors.
Fragments OK. Code, commits, PR bodies: normal prose.

## Tok: Cache Alignment
Provider prompt caching (Anthropic/OpenAI/Gemini) only hits when the prefix is
byte-identical to a prior request, so:
- Never re-paste or reorder earlier file contents/instructions already in context — reference them.
- Don't put timestamps, request IDs, or other per-turn noise early; if unavoidable, put it last.
- Treat conversation history as append-only within a turn — editing or reordering earlier content breaks the cached prefix for every token after it.
<!-- tok-skill: end -->
`

const ultraBlock = `
<!-- tok-skill: start — remove this block to disable -->
## Tok: Response Brevity
Be terse. Drop: articles, pleasantries, hedging, filler.
Never drop: not/never/no, numbers, technical terms, exact errors.
Fragments OK. Code, commits, PR bodies: normal prose.

## Tok: Cache Alignment
Provider prompt caching (Anthropic/OpenAI/Gemini) only hits when the prefix is
byte-identical to a prior request, so:
- Never re-paste or reorder earlier file contents/instructions already in context — reference them.
- Don't put timestamps, request IDs, or other per-turn noise early; if unavoidable, put it last.
- Treat conversation history as append-only within a turn — editing or reordering earlier content breaks the cached prefix for every token after it.

## Tok: Minimal Diff
Before writing code, stop at the first option that holds: reuse existing
repo code, use the standard library, write one line, or write the minimum
that works. Never skip validation, error handling, or security for brevity.
<!-- tok-skill: end -->
`

func blockFor(mode string) string {
	switch mode {
	case "lite":
		return liteBlock
	case "ultra":
		return ultraBlock
	case "off":
		return ""
	default:
		return fullBlock
	}
}

// Inject writes the tok skill block (sized per mode) into the agent
// instruction file at path. If the block already exists it replaces it in-place.
// mode="off" removes the block entirely.
func Inject(path, mode string) error {
	if mode == "off" {
		return Remove(path)
	}
	block := blockFor(mode)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(strings.TrimSpace(block)+"\n"), 0644)
	}
	if err != nil {
		return err
	}

	content := string(data)
	start := strings.Index(content, blockStart)
	end := strings.Index(content, blockEnd)
	if start != -1 && end != -1 {
		// Replace existing block
		content = content[:start] + strings.TrimLeft(block, "\n") + content[end+len(blockEnd):]
	} else {
		// Append
		content = strings.TrimRight(content, "\n") + "\n\n" + strings.TrimSpace(block) + "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// Remove strips the tok skill block from path.
func Remove(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	content := string(data)
	start := strings.Index(content, blockStart)
	end := strings.Index(content, blockEnd)
	if start == -1 || end == -1 {
		return nil
	}
	content = content[:start] + content[end+len(blockEnd)+1:]
	return os.WriteFile(path, []byte(strings.TrimRight(content, "\n")+"\n"), 0644)
}

// HasBlock reports whether path currently contains a tok skill block.
func HasBlock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), blockStart)
}

// DefaultFiles returns the agent instruction files to inject into for the cwd.
func DefaultFiles(copilot bool) []string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	var candidates []string

	if copilot && home != "" {
		// Global VS Code Copilot instructions — highest-value target.
		candidates = append(candidates, filepath.Join(home, ".copilot", "copilot-instructions.md"))
	}

	// Repo-level files present on disk.
	for _, name := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		p := filepath.Join(cwd, name)
		if _, err := os.Stat(p); err == nil {
			candidates = append(candidates, p)
		}
	}

	// Global Claude.md when no repo-level files found.
	if home != "" {
		globalClaude := filepath.Join(home, ".claude", "CLAUDE.md")
		if _, err := os.Stat(globalClaude); err == nil {
			candidates = append(candidates, globalClaude)
		}
	}

	if len(candidates) == 0 {
		candidates = append(candidates, filepath.Join(cwd, "CLAUDE.md"))
	}
	return candidates
}

// InjectedFiles scans the usual instruction-file locations (cwd + home) and
// returns the ones that currently contain a tok skill block. Used by
// `tok mode` to re-inject at the new intensity without requiring --copilot.
func InjectedFiles() []string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	var all []string
	if home != "" {
		all = append(all,
			filepath.Join(home, ".copilot", "copilot-instructions.md"),
			filepath.Join(home, ".claude", "CLAUDE.md"),
		)
	}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		all = append(all, filepath.Join(cwd, name))
	}

	var found []string
	for _, p := range all {
		if HasBlock(p) {
			found = append(found, p)
		}
	}
	return found
}

