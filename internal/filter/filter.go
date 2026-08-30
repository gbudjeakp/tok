// Package filter routes commands to their output compressors and records savings.
package filter

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gbudjeakp/tok/internal/analytics"
)

// Result holds both the filtered output and raw metrics.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	OrigLen  int
	OutLen   int
	FilterID string
}

type filterFn func(cmd string, args []string) (*Result, error)

// registry maps the first argument (command name) to a filter function.
// Entries are checked in order; first match wins.
var registry []entry

type entry struct {
	match  func(cmd string, args []string) bool
	filter filterFn
}

func register(match func(cmd, arg0 string) bool, fn filterFn) {
	registry = append(registry, entry{
		match: func(cmd string, args []string) bool {
			a0 := ""
			if len(args) > 0 {
				a0 = args[0]
			}
			return match(cmd, a0)
		},
		filter: fn,
	})
}

// runnerPrefixes are wrappers tok strips before matching a filter.
// e.g. "uv run pytest -v" → cmd=pytest args=["-v"]
var runnerPrefixes = [][]string{
	{"uv", "run"},
	{"poetry", "run"},
	{"pdm", "run"},
	{"pipenv", "run"},
	{"rye", "run"},
	{"hatch", "run"},
	{"npx"},
	{"bunx"},
	{"pnpm", "exec"},
	{"yarn", "exec"},
}

// resolve strips a leading path component from cmd and unwraps runner prefixes,
// returning the effective (cmd, args) for filter lookup.
func resolve(cmd string, args []string) (string, []string) {
	// Strip absolute path: /usr/bin/git → git
	cmd = filepath.Base(cmd)

	// Unwrap runner prefixes: "uv run pytest -v" → ("pytest", ["-v"])
	for _, prefix := range runnerPrefixes {
		if cmd != prefix[0] {
			continue
		}
		rest := args
		matched := true
		for _, p := range prefix[1:] {
			if len(rest) == 0 || rest[0] != p {
				matched = false
				break
			}
			rest = rest[1:]
		}
		if matched && len(rest) > 0 {
			// Skip runner flags and their values (e.g. "--python 3.12").
			for len(rest) > 0 {
				if strings.HasPrefix(rest[0], "-") {
					rest = rest[1:]
					// If next token looks like a flag value (not a command), skip it too.
					if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") && !looksLikeCommand(rest[0]) {
						rest = rest[1:]
					}
				} else {
					break
				}
			}
			if len(rest) > 0 {
				return filepath.Base(rest[0]), rest[1:]
			}
		}
	}
	return cmd, args
}

// looksLikeCommand returns true if s looks like an executable name rather than
// a flag value (version string, path, number).
func looksLikeCommand(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Version strings start with a digit; paths contain slashes or dots mid-word.
	if s[0] >= '0' && s[0] <= '9' {
		return false
	}
	if strings.ContainsAny(s, "/") {
		return false
	}
	return true
}

// smallOutputThreshold: outputs shorter than this pass through as-is.
// Compressing tiny output hurts readability while saving almost nothing.
const smallOutputThreshold = 800

// RunCommand executes cmd with args through the appropriate filter,
// prints the compressed output, and records savings. Returns the exit code.
func RunCommand(cmd string, args []string) int {
	effectiveCmd, effectiveArgs := resolve(cmd, args)
	for _, e := range registry {
		if e.match(effectiveCmd, effectiveArgs) {
			res, err := e.filter(effectiveCmd, effectiveArgs)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			out := logfold(res.Stdout)
			// Session memory: return a one-liner if the output is unchanged.
			if cached := checkCache(effectiveCmd, effectiveArgs, out); cached != "" {
				fmt.Print(cached)
				if res.Stderr != "" {
					fmt.Fprint(os.Stderr, res.Stderr)
				}
				return res.ExitCode
			}
			if out != "" {
				fmt.Print(out)
			}
			if res.Stderr != "" {
				fmt.Fprint(os.Stderr, res.Stderr)
			}
			analytics.Record(cmd, res.FilterID, res.OrigLen, res.OutLen)
			return res.ExitCode
		}
	}
	// No specific filter — try JSON compression as fallback.
	stdout, stderr, ec := run(effectiveCmd, effectiveArgs)
	if len(stdout) >= smallOutputThreshold {
		if crushed, ok := SmashJSON(stdout); ok {
			fmt.Print(crushed)
			if stderr != "" {
				fmt.Fprint(os.Stderr, stderr)
			}
			analytics.Record(cmd, "json", len(stdout), len(crushed))
			return ec
		}
	}
	// Raw passthrough.
	if stdout != "" {
		fmt.Print(stdout)
	}
	if stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}
	return ec
}

// Filterable reports whether cmd (or a runner wrapping it) has a registered filter,
// without executing the command. Used by tok check.
func Filterable(cmd string, args []string) (effectiveCmd string, matched bool) {
	effectiveCmd, effectiveArgs := resolve(cmd, args)
	for _, e := range registry {
		if e.match(effectiveCmd, effectiveArgs) {
			return effectiveCmd, true
		}
	}
	return effectiveCmd, false
}


// passthrough runs the command unchanged and records no savings.
func passthrough(cmd string, args []string) int {
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// run executes a command and captures stdout + stderr separately.
func run(name string, args []string) (stdout, stderr string, exitCode int) {
	var outBuf, errBuf bytes.Buffer
	c := exec.Command(name, args...)
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err := c.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// FilterOnly runs cmd through the filter engine and returns the result without
// printing or recording analytics. Returns nil if no filter matches.
func FilterOnly(cmd string, args []string) *Result {
	effectiveCmd, effectiveArgs := resolve(cmd, args)
	for _, e := range registry {
		if e.match(effectiveCmd, effectiveArgs) {
			res, err := e.filter(effectiveCmd, effectiveArgs)
			if err != nil {
				return nil
			}
			return res
		}
	}
	// Fallback: try generic JSON compression.
	stdout, stderr, ec := run(effectiveCmd, effectiveArgs)
	if len(stdout) >= smallOutputThreshold {
		if crushed, ok := SmashJSON(stdout); ok {
			return &Result{
				Stdout: crushed, Stderr: stderr, ExitCode: ec,
				OrigLen: len(stdout), OutLen: len(crushed), FilterID: "json",
			}
		}
	}
	return nil
}

// RawRun executes cmd and captures its combined output without any filtering.
func RawRun(cmd string, args []string) (string, int) {
	var buf bytes.Buffer
	c := exec.Command(cmd, args...)
	c.Stdout = &buf
	c.Stderr = &buf
	exitCode := 0
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return buf.String(), exitCode
}
