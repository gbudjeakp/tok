package filter

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	register(func(cmd, _ string) bool {
		switch cmd {
		case "ruff", "eslint", "tsc", "golangci-lint", "biome", "oxlint":
			return true
		}
		return false
	}, filterLint)

	register(matchBuild, filterBuild)
}

func matchBuild(cmd, arg0 string) bool {
	switch cmd {
	case "cargo":
		return arg0 == "build" || arg0 == "clippy" || arg0 == "check"
	case "tsc":
		return true
	case "go":
		return arg0 == "build" || arg0 == "vet"
	}
	return false
}

func filterBuild(cmd string, args []string) (*Result, error) {
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)
	combined := stdout + stderr

	// Keep only error/warning lines; strip "Compiling X", "Checking X", progress noise.
	out := buildErrors(combined)
	if out == "" {
		// Clean build — single confirmation line.
		out = "ok\n"
	}
	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: cmd + "-build"}, nil
}

var reCargoError = regexp.MustCompile(`(?i)^(error(\[E\d+\])?|warning):`)

func buildErrors(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var keep []string
	inBlock := false

	for _, l := range lines {
		// Cargo/Go error block: keep "error:" lines and their context
		if reCargoError.MatchString(strings.TrimSpace(l)) {
			inBlock = true
			keep = append(keep, l)
			continue
		}
		// File:line context inside an error block
		if inBlock && (strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "\t") || strings.HasPrefix(l, " -->")) {
			keep = append(keep, l)
			continue
		}
		inBlock = false
	}
	if len(keep) == 0 {
		return ""
	}
	return strings.Join(keep, "\n") + "\n"
}

func filterLint(cmd string, args []string) (*Result, error) {
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)
	combined := stdout + stderr

	out := compactLint(combined)
	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: cmd + "-lint"}, nil
}

// compactLint groups errors/warnings by file and collapses passing files.
func compactLint(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 15 {
		return output
	}

	// Group: file → []message
	fileErrors := map[string][]string{}
	var order []string

	// Common pattern: "file.go:line:col: message"
	reFileLine := regexp.MustCompile(`^([^:]+\.(?:go|ts|tsx|js|jsx|py|rs)):(\d+)(?::\d+)?:?\s+(.+)$`)

	for _, l := range lines {
		if m := reFileLine.FindStringSubmatch(l); m != nil {
			file := m[1]
			msg := fmt.Sprintf("  :%s: %s", m[2], truncate(m[3], 100))
			if _, ok := fileErrors[file]; !ok {
				order = append(order, file)
			}
			fileErrors[file] = append(fileErrors[file], msg)
		}
	}

	if len(order) == 0 {
		return output
	}

	var sb strings.Builder
	totalErrors := 0
	for _, f := range order {
		errs := fileErrors[f]
		totalErrors += len(errs)
		sb.WriteString(f + "\n")
		limit := 4
		for i, e := range errs {
			if i >= limit {
				sb.WriteString(fmt.Sprintf("  ... +%d more\n", len(errs)-limit))
				break
			}
			sb.WriteString(e + "\n")
		}
	}
	sb.WriteString(fmt.Sprintf("\n%d errors in %d files\n", totalErrors, len(order)))
	return sb.String()
}

// Summary runs any command and applies a heuristic summary — keeps error/warning lines.
func Summary(cmd string, args []string) (*Result, error) {
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)
	combined := stdout + stderr
	out := heuristicSummary(combined)
	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: "summary"}, nil
}

var reImportant = regexp.MustCompile(`(?i)(error|warn|fail|panic|fatal|exception|FAILED|PASS|success|done|complete)`)

func heuristicSummary(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 20 {
		return output
	}

	var keep []string
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if reImportant.MatchString(l) {
			keep = append(keep, l)
		}
	}

	if len(keep) == 0 {
		// Nothing found — show first+last 5 lines
		n := 5
		if len(lines) <= n*2 {
			return output
		}
		result := lines[:n]
		result = append(result, fmt.Sprintf("... (%d lines omitted) ...", len(lines)-n*2))
		result = append(result, lines[len(lines)-n:]...)
		return strings.Join(result, "\n") + "\n"
	}

	return strings.Join(keep, "\n") + "\n"
}
