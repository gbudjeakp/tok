package filter

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	register(matchTest, filterTest)
}

func matchTest(cmd, arg0 string) bool {
	switch cmd {
	case "go", "cargo", "pytest", "python":
		return true
	case "npm", "yarn", "pnpm", "bun":
		return arg0 == "test" || arg0 == "run"
	}
	return false
}

func filterTest(cmd string, args []string) (*Result, error) {
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)

	var out, id string
	switch cmd {
	case "go":
		if len(args) > 0 && args[0] == "test" {
			out, id = filterGoTest(stdout, stderr), "go-test"
		} else {
			out, id = stdout+stderr, "go-pass"
		}
	case "cargo":
		if len(args) > 0 && args[0] == "test" {
			out, id = filterCargoTest(stdout, stderr), "cargo-test"
		} else {
			out, id = stdout+stderr, "cargo-pass"
		}
	case "pytest", "python":
		out, id = filterPytest(stdout, stderr), "pytest"
	case "npm", "yarn", "pnpm", "bun":
		out, id = filterNpmTest(stdout, stderr), "npm-test"
	default:
		return &Result{Stdout: stdout, Stderr: stderr, ExitCode: ec, OrigLen: orig, OutLen: orig, FilterID: "test-pass"}, nil
	}

	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: id}, nil
}

var (
	reGoOk      = regexp.MustCompile(`(?m)^ok\s+`)
	reGoFailPkg = regexp.MustCompile(`(?m)^FAIL\s+`)
)

func filterGoTest(stdout, stderr string) string {
	combined := stdout + stderr
	lines := strings.Split(combined, "\n")

	var failures []string
	var summary []string
	inFailure := false
	failCount, passCount := 0, 0

	for _, l := range lines {
		if strings.HasPrefix(l, "--- FAIL:") {
			inFailure = true
			failCount++
			failures = append(failures, l)
			continue
		}
		if strings.HasPrefix(l, "--- PASS:") {
			inFailure = false
			passCount++
			continue
		}
		if inFailure && (strings.HasPrefix(l, "    ") || strings.HasPrefix(l, "\t")) {
			// Include failure detail lines (file:line: message)
			failures = append(failures, l)
			continue
		}
		if inFailure {
			inFailure = false
		}
		if reGoOk.MatchString(l) || reGoFailPkg.MatchString(l) {
			summary = append(summary, l)
		}
	}

	total := failCount + passCount
	if total == 0 {
		return combined
	}

	var sb strings.Builder
	if failCount > 0 {
		sb.WriteString(fmt.Sprintf("FAILED: %d/%d tests\n", failCount, total))
		for _, f := range failures {
			sb.WriteString(f + "\n")
		}
	} else {
		sb.WriteString(fmt.Sprintf("PASS: %d tests\n", total))
	}
	for _, s := range summary {
		sb.WriteString(s + "\n")
	}
	return sb.String()
}

var (
	reCargoFail    = regexp.MustCompile(`(?m)^test .+ \.\.\. FAILED$`)
	reCargoResult  = regexp.MustCompile(`(?m)^test result:.*$`)
	reCargoFailout = regexp.MustCompile(`(?m)^---- .+ stdout ----`)
)

func filterCargoTest(stdout, stderr string) string {
	combined := stdout + stderr
	lines := strings.Split(combined, "\n")

	var failures []string
	var resultLine string
	inFailDetails := false

	for _, l := range lines {
		if reCargoFail.MatchString(l) {
			failures = append(failures, l)
			continue
		}
		if reCargoFailout.MatchString(l) {
			inFailDetails = true
			failures = append(failures, l)
			continue
		}
		if inFailDetails {
			if l == "" {
				inFailDetails = false
			} else {
				failures = append(failures, l)
			}
			continue
		}
		if reCargoResult.MatchString(l) {
			resultLine = l
		}
	}

	if len(failures) == 0 && resultLine != "" {
		return resultLine + "\n"
	}
	var sb strings.Builder
	for _, f := range failures {
		sb.WriteString(f + "\n")
	}
	if resultLine != "" {
		sb.WriteString(resultLine + "\n")
	}
	if sb.Len() == 0 {
		return combined
	}
	return sb.String()
}

var (
	rePytestFail  = regexp.MustCompile(`(?m)^FAILED `)
	rePytestShort = regexp.MustCompile(`(?m)^=+ short test summary`)
	rePytestEnd   = regexp.MustCompile(`(?m)^=+ \d+ (failed|passed|error)`)
)

func filterPytest(stdout, stderr string) string {
	combined := stdout + stderr
	lines := strings.Split(combined, "\n")

	var failures []string
	var summary []string
	inShort := false

	for _, l := range lines {
		if rePytestShort.MatchString(l) {
			inShort = true
			continue
		}
		if inShort {
			if rePytestFail.MatchString(l) {
				failures = append(failures, l)
			} else if l == "" {
				inShort = false
			}
			continue
		}
		if rePytestEnd.MatchString(l) {
			summary = append(summary, l)
		}
	}

	if len(failures) == 0 && len(summary) == 0 {
		return combined
	}

	var sb strings.Builder
	for _, f := range failures {
		sb.WriteString(f + "\n")
	}
	for _, s := range summary {
		sb.WriteString(s + "\n")
	}
	return sb.String()
}

var (
	reJestFail  = regexp.MustCompile(`(?m)^\s+● `)
	reJestSuite = regexp.MustCompile(`(?m)^Test Suites:|^Tests:`)
)

func filterNpmTest(stdout, stderr string) string {
	combined := stdout + stderr
	lines := strings.Split(combined, "\n")

	var failures []string
	var summary []string
	inFailure := false

	for _, l := range lines {
		if reJestFail.MatchString(l) {
			inFailure = true
			failures = append(failures, l)
			continue
		}
		if inFailure {
			if l == "" {
				inFailure = false
			} else {
				failures = append(failures, l)
			}
			continue
		}
		if reJestSuite.MatchString(l) {
			summary = append(summary, l)
		}
	}

	if len(failures) == 0 && len(summary) == 0 {
		return combined
	}

	var sb strings.Builder
	for _, f := range failures {
		sb.WriteString(f + "\n")
	}
	for _, s := range summary {
		sb.WriteString(s + "\n")
	}
	return sb.String()
}
