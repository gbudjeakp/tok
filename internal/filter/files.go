package filter

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	register(func(cmd, _ string) bool {
		switch cmd {
		case "ls", "find", "grep", "rg", "cat", "head", "tail":
			return true
		}
		return false
	}, filterFiles)
}

func filterFiles(cmd string, args []string) (*Result, error) {
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)

	var out, id string
	switch cmd {
	case "ls":
		out, id = filterLs(stdout, args), "ls"
	case "find":
		out, id = filterFind(stdout), "find"
	case "grep", "rg":
		out, id = filterGrep(stdout), "grep"
	default:
		// Try JSON compression for cat/head/tail — agents often cat JSON files
		if cmd == "cat" || cmd == "head" {
			if crushed, ok := SmashJSON(stdout); ok {
				return &Result{Stdout: crushed, ExitCode: ec, OrigLen: orig, OutLen: len(crushed), FilterID: "json"}, nil
			}
		}
		return &Result{Stdout: stdout, Stderr: stderr, ExitCode: ec, OrigLen: orig, OutLen: orig, FilterID: "files-pass"}, nil
	}

	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: id}, nil
}

func filterLs(output string, args []string) string {
	// Build a tree-style view grouped by directory prefix
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 8 {
		return output
	}

	// If long format (-l), strip permissions/ownership, keep name+size
	isLong := false
	for _, a := range args {
		if strings.Contains(a, "l") && strings.HasPrefix(a, "-") {
			isLong = true
		}
	}

	if isLong {
		return filterLsLong(lines)
	}

	// Simple compact: group into columns of 4
	var sb strings.Builder
	for i, l := range lines {
		if i > 0 && i%4 == 0 {
			sb.WriteString("\n")
		} else if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(l)
	}
	sb.WriteString(fmt.Sprintf("\n(%d entries)\n", len(lines)))
	return sb.String()
}

var reLsLong = regexp.MustCompile(`^([d\-lrwxst]{10})\s+\d+\s+\S+\s+\S+\s+(\d+[KMG]?)\s+\S+\s+\S+\s+\S+\s+(.+)$`)

func filterLsLong(lines []string) string {
	var sb strings.Builder
	for _, l := range lines {
		if strings.HasPrefix(l, "total") {
			continue
		}
		if m := reLsLong.FindStringSubmatch(l); m != nil {
			// perms  size  name
			sb.WriteString(fmt.Sprintf("%-12s %6s  %s\n", m[1], m[2], m[3]))
		} else {
			sb.WriteString(l + "\n")
		}
	}
	return sb.String()
}

func filterFind(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 20 {
		return output
	}

	// Group by parent directory
	dirs := map[string][]string{}
	var order []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		dir := filepath.Dir(l)
		if _, ok := dirs[dir]; !ok {
			order = append(order, dir)
		}
		dirs[dir] = append(dirs[dir], filepath.Base(l))
	}

	var sb strings.Builder
	for _, d := range order {
		files := dirs[d]
		if len(files) == 1 {
			sb.WriteString(filepath.Join(d, files[0]) + "\n")
		} else {
			sb.WriteString(fmt.Sprintf("%s/ (%d files: %s)\n", d, len(files), joinShort(files, 3)))
		}
	}
	return sb.String()
}

// filterGrep groups matches by file and truncates long lines.
func filterGrep(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 15 {
		return output
	}

	fileMatches := map[string][]string{}
	var order []string

	reGrepLine := regexp.MustCompile(`^([^:]+):(\d+):(.*)$`)
	for _, l := range lines {
		if m := reGrepLine.FindStringSubmatch(l); m != nil {
			file := m[1]
			match := fmt.Sprintf("  :%s: %s", m[2], truncate(m[3], 80))
			if _, ok := fileMatches[file]; !ok {
				order = append(order, file)
			}
			fileMatches[file] = append(fileMatches[file], match)
		} else {
			// Binary match or other output
			if l != "" {
				order = append(order, l)
				fileMatches[l] = nil
			}
		}
	}

	var sb strings.Builder
	for _, f := range order {
		matches := fileMatches[f]
		if matches == nil {
			sb.WriteString(f + "\n")
			continue
		}
		sb.WriteString(f + ":\n")
		limit := 5
		for i, m := range matches {
			if i >= limit {
				sb.WriteString(fmt.Sprintf("  ... +%d more\n", len(matches)-limit))
				break
			}
			sb.WriteString(m + "\n")
		}
	}

	// Print total count
	total := 0
	for _, m := range fileMatches {
		total += len(m)
	}
	sb.WriteString(fmt.Sprintf("(%d matches in %d files)\n", total, len(order)))
	return sb.String()
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// CompactJSON shows JSON structure without values — just keys and nesting.
func CompactJSON(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= 20 {
		return output
	}
	var sb strings.Builder
	valueRe := regexp.MustCompile(`"[^"]+": (?:"[^"]*"|[\d.]+|true|false|null)(,?)$`)
	for _, l := range lines {
		// Show structural lines, collapse simple value lines
		trimmed := strings.TrimSpace(l)
		if valueRe.MatchString(trimmed) {
			// Replace value with type hint
			l = valueRe.ReplaceAllStringFunc(l, func(match string) string {
				if strings.Contains(match, `"`) {
					return valueRe.ReplaceAllString(match, `"…"`)
				}
				return match
			})
		}
		sb.WriteString(l + "\n")
	}
	return sb.String()
}

// unused but available for file read filtering
var _ = os.ReadFile
