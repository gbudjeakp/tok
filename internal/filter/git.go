package filter

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	register(func(cmd, _ string) bool { return cmd == "git" }, filterGit)
}

func filterGit(cmd string, args []string) (*Result, error) {
	if len(args) == 0 {
		return nil, nil
	}
	sub := args[0]
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)

	var out string
	var id string

	switch sub {
	case "status":
		out, id = gitStatus(stdout), "git-status"
	case "log":
		// Re-run with our own format regardless of what flags the agent passed.
		out, id = gitLogForced(args[1:]), "git-log"
	case "diff", "show":
		out, id = gitDiff(stdout), "git-diff"
	case "add":
		out, id = "ok\n", "git-add"
	case "commit":
		out, id = gitCommit(stdout, stderr), "git-commit"
	case "push":
		out, id = gitPush(stdout, stderr), "git-push"
	case "pull":
		out, id = gitPull(stdout, stderr), "git-pull"
	default:
		return &Result{Stdout: stdout, Stderr: stderr, ExitCode: ec, OrigLen: orig, OutLen: orig, FilterID: "git-pass"}, nil
	}

	return &Result{Stdout: out, Stderr: "", ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: id}, nil
}

var (
	reBranch   = regexp.MustCompile(`(?m)^(?:On branch|HEAD detached at) (.+)$`)
	reModified = regexp.MustCompile(`(?m)^\s+modified:\s+(.+)$`)
	reDeleted  = regexp.MustCompile(`(?m)^\s+deleted:\s+(.+)$`)
	reNewFile  = regexp.MustCompile(`(?m)^\s+new file:\s+(.+)$`)
	reStagedSec = regexp.MustCompile(`(?s)Changes to be committed:(.+?)(?:\n\n|\z)`)
	reNotStaged = regexp.MustCompile(`(?s)Changes not staged for commit:(.+?)(?:\n\n|\z)`)
	reUntracked = regexp.MustCompile(`(?s)Untracked files:(.+?)(?:\n\n|\z)`)
)

func gitStatus(out string) string {
	branch := "HEAD"
	if m := reBranch.FindStringSubmatch(out); m != nil {
		branch = m[1]
	}

	var parts []string

	if m := reStagedSec.FindStringSubmatch(out); m != nil {
		files := extractFiles(m[1])
		if len(files) > 0 {
			parts = append(parts, fmt.Sprintf("staged(%d): %s", len(files), joinShort(files, 3)))
		}
	}
	if m := reNotStaged.FindStringSubmatch(out); m != nil {
		files := extractFiles(m[1])
		if len(files) > 0 {
			parts = append(parts, fmt.Sprintf("modified(%d): %s", len(files), joinShort(files, 3)))
		}
	}
	if m := reUntracked.FindStringSubmatch(out); m != nil {
		lines := strings.Split(strings.TrimSpace(m[1]), "\n")
		var files []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "(") {
				files = append(files, l)
			}
		}
		if len(files) > 0 {
			parts = append(parts, fmt.Sprintf("untracked(%d): %s", len(files), joinShort(files, 3)))
		}
	}

	if len(parts) == 0 {
		return branch + " | clean\n"
	}
	return branch + " | " + strings.Join(parts, " | ") + "\n"
}

func extractFiles(section string) []string {
	var files []string
	for _, re := range []*regexp.Regexp{reModified, reDeleted, reNewFile} {
		for _, m := range re.FindAllStringSubmatch(section, -1) {
			files = append(files, strings.TrimSpace(m[1]))
		}
	}
	return files
}

func joinShort(files []string, max int) string {
	if len(files) <= max {
		return strings.Join(files, ", ")
	}
	return strings.Join(files[:max], ", ") + fmt.Sprintf(" +%d", len(files)-max)
}

// gitLogForced re-runs git log with a compact format, stripping format flags.
func gitLogForced(userArgs []string) string {
	// Strip any --format/--pretty/--oneline flags the caller passed.
	var passArgs []string
	for i := 0; i < len(userArgs); i++ {
		a := userArgs[i]
		if strings.HasPrefix(a, "--format") || strings.HasPrefix(a, "--pretty") ||
			a == "--oneline" || a == "--no-decorate" || a == "--decorate" {
			if !strings.Contains(a, "=") {
				i++ // skip next value token too
			}
			continue
		}
		passArgs = append(passArgs, a)
	}
	args := append([]string{"log", "--format=%h %s (%an, %ar)", "--no-decorate"}, passArgs...)
	stdout, stderr, _ := run("git", args)
	if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) != "" {
		return stderr
	}
	return stdout
}

func gitDiff(out string) string {
	lines := strings.Split(out, "\n")
	var result []string
	contextCount := 0
	const maxContext = 2

	for _, l := range lines {
		// Always keep headers and changed lines
		if strings.HasPrefix(l, "diff ") || strings.HasPrefix(l, "--- ") ||
			strings.HasPrefix(l, "+++ ") || strings.HasPrefix(l, "@@ ") {
			contextCount = 0
			result = append(result, l)
			continue
		}
		if strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") {
			contextCount = 0
			result = append(result, l)
			continue
		}
		// Context lines — keep up to maxContext
		if contextCount < maxContext {
			result = append(result, l)
		}
		contextCount++
	}
	return strings.Join(result, "\n") + "\n"
}

var reCommitHash = regexp.MustCompile(`\[([^\]]+)\s+([a-f0-9]+)\]`)

func gitCommit(stdout, stderr string) string {
	combined := stdout + stderr
	if m := reCommitHash.FindStringSubmatch(combined); m != nil {
		return fmt.Sprintf("ok %s\n", m[2][:7])
	}
	// Fallback: show first non-empty line
	for _, l := range strings.Split(combined, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			return "ok " + l + "\n"
		}
	}
	return "ok\n"
}

func gitPush(stdout, stderr string) string {
	combined := stdout + stderr
	// Look for branch info: "main -> main"
	if strings.Contains(combined, "->") {
		for _, l := range strings.Split(combined, "\n") {
			if strings.Contains(l, "->") {
				branch := strings.TrimSpace(l)
				return fmt.Sprintf("ok %s\n", branch)
			}
		}
	}
	if strings.Contains(combined, "up-to-date") || strings.Contains(combined, "up to date") {
		return "ok (already up to date)\n"
	}
	return "ok\n"
}

func gitPull(stdout, stderr string) string {
	combined := stdout + stderr
	if strings.Contains(combined, "up to date") || strings.Contains(combined, "up-to-date") {
		return "ok (already up to date)\n"
	}
	// Extract summary: "3 files changed, +10 -2"
	reChanged := regexp.MustCompile(`(\d+ files? changed[^\n]+)`)
	if m := reChanged.FindStringSubmatch(combined); m != nil {
		return fmt.Sprintf("ok %s\n", strings.TrimSpace(m[1]))
	}
	return "ok\n"
}

// Rewrite returns the tok-rewritten command if it should be filtered, or "" if passthrough.
// Used by the Claude Code hook.
func Rewrite(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}
	cmd := parts[0]
	args := parts[1:]

	effectiveCmd, effectiveArgs := resolve(cmd, args)
	for _, e := range registry {
		if e.match(effectiveCmd, effectiveArgs) {
			return "tok " + command
		}
	}
	return ""
}
