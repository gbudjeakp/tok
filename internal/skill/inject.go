// Package skill injects tok's brevity and cache-alignment hints into agent instruction files.
package skill

import (
	"os"
	"path/filepath"
	"strings"
)

const skillBlock = `
<!-- tok-skill: start — remove this block to disable -->
## Tok: Response Brevity
Be terse. Drop: articles, pleasantries, hedging, filler.
Never drop: not/never/no, numbers, technical terms, exact errors.
Fragments OK. Code, commits, PR bodies: normal prose.

## Tok: Cache Alignment
Place stable content (file contents, instructions) before volatile content
(tool results, current errors) so provider KV-cache prefixes stay stable.
Never restate what a tool result just showed — reference it directly.
<!-- tok-skill: end -->
`

// Inject writes the tok skill block into the agent instruction file at path.
// If the block already exists it replaces it in-place.
func Inject(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(strings.TrimSpace(skillBlock)+"\n"), 0644)
	}
	if err != nil {
		return err
	}

	content := string(data)
	start := strings.Index(content, "<!-- tok-skill: start")
	end := strings.Index(content, "<!-- tok-skill: end -->")
	if start != -1 && end != -1 {
		// Replace existing block
		content = content[:start] + strings.TrimLeft(skillBlock, "\n") + content[end+len("<!-- tok-skill: end -->"):]
	} else {
		// Append
		content = strings.TrimRight(content, "\n") + "\n\n" + strings.TrimSpace(skillBlock) + "\n"
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
	start := strings.Index(content, "<!-- tok-skill: start")
	end := strings.Index(content, "<!-- tok-skill: end -->")
	if start == -1 || end == -1 {
		return nil
	}
	content = content[:start] + content[end+len("<!-- tok-skill: end -->")+1:]
	return os.WriteFile(path, []byte(strings.TrimRight(content, "\n")+"\n"), 0644)
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
