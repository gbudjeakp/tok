package filter

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	register(matchPkg, filterPkg)
}

func matchPkg(cmd, arg0 string) bool {
	switch cmd {
	case "pip", "pip3":
		return arg0 == "list" || arg0 == "outdated" || arg0 == "show"
	case "npm", "yarn", "pnpm", "bun":
		return arg0 == "list" || arg0 == "ls" || arg0 == "outdated" || arg0 == "install" || arg0 == "add" || arg0 == "remove"
	case "brew":
		return arg0 == "list" || arg0 == "outdated" || arg0 == "info"
	case "bundle":
		return arg0 == "install" || arg0 == "list"
	}
	return false
}

func filterPkg(cmd string, args []string) (*Result, error) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)

	var out, id string

	switch {
	case (cmd == "pip" || cmd == "pip3") && sub == "list":
		out, id = pipList(stdout), "pip-list"
	case (cmd == "pip" || cmd == "pip3") && sub == "outdated":
		out, id = stdout, "pip-outdated" // already compact
	case cmd == "brew" && sub == "list":
		out, id = brewList(stdout), "brew-list"
	case cmd == "brew" && sub == "outdated":
		out, id = stdout, "brew-outdated"
	case (cmd == "npm" || cmd == "yarn" || cmd == "pnpm" || cmd == "bun") && (sub == "list" || sub == "ls"):
		out, id = npmList(stdout), "npm-list"
	case (cmd == "npm" || cmd == "yarn" || cmd == "pnpm" || cmd == "bun") && (sub == "install" || sub == "add" || sub == "remove"):
		out, id = pkgInstallSummary(stdout+stderr, cmd, sub), "pkg-install"
	case cmd == "bundle" && sub == "install":
		out, id = bundleInstall(stdout+stderr), "bundle-install"
	default:
		return &Result{Stdout: stdout, Stderr: stderr, ExitCode: ec, OrigLen: orig, OutLen: orig, FilterID: "pkg-pass"}, nil
	}

	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: id}, nil
}

// pipList: keep package name+version only, strip header and separator lines.
var rePipHeader = regexp.MustCompile(`^(?:Package|---|\s*$)`)

func pipList(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var pkgs []string
	for _, l := range lines {
		if rePipHeader.MatchString(l) {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) >= 2 {
			pkgs = append(pkgs, fields[0]+"=="+fields[1])
		}
	}
	if len(pkgs) == 0 {
		return output
	}
	return strings.Join(pkgs, "  ") + fmt.Sprintf("\n(%d packages)\n", len(pkgs))
}

// brewList: compact columns.
func brewList(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 10 {
		return output
	}
	return strings.Join(lines, "  ") + fmt.Sprintf("\n(%d formulae)\n", len(lines))
}

// npmList: strip tree decorations, keep name@version.
var reNpmPkg = regexp.MustCompile(`(@?[a-zA-Z0-9@/_.-]+@[\d.]+[^\s]*)`)

func npmList(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 15 {
		return output
	}
	var pkgs []string
	for _, l := range lines {
		if m := reNpmPkg.FindString(l); m != "" {
			pkgs = append(pkgs, m)
		}
	}
	if len(pkgs) == 0 {
		return output
	}
	return strings.Join(pkgs, "\n") + fmt.Sprintf("\n(%d packages)\n", len(pkgs))
}

// pkgInstallSummary: strip per-package download/progress, keep final summary.
var reInstallProgress = regexp.MustCompile(`(?i)(downloading|fetching|resolving|lockfile|already installed|^added \d|^removed \d|packages in|done in|^up to date|Progress:|\[=)`)

func pkgInstallSummary(output, cmd, sub string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var keep []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if reInstallProgress.MatchString(t) {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		// Nothing recognisable — show last 3 lines as summary.
		if len(lines) > 3 {
			return strings.Join(lines[len(lines)-3:], "\n") + "\n"
		}
		return output
	}
	return strings.Join(keep, "\n") + "\n"
}

// bundleInstall: strip "Using X (1.2.3)" lines, keep installs and summary.
var reUsingLine = regexp.MustCompile(`^Using `)

func bundleInstall(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var keep []string
	for _, l := range lines {
		if !reUsingLine.MatchString(l) {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return output
	}
	return strings.Join(keep, "\n") + "\n"
}
