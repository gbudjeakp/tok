package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"tok/internal/filter"
)

var ansiRe = regexp.MustCompile(`\033\[[0-9;]*m`)

func newProveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prove <command> [args...]",
		Short: "Show raw vs tok output with token savings side-by-side",
		Example: `  tok prove git log -5
  tok prove git status
  tok prove docker ps
  tok prove go test ./...`,
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProve(args[0], args[1:])
		},
	}
}

func runProve(cmd string, args []string) error {
	raw, rawExit := filter.RawRun(cmd, args)
	res := filter.FilterOnly(cmd, args)

	rawToks := estimateToks(raw)
	if res == nil {
		fmt.Printf("tok has no filter for %q — no savings.\n", cmd)
		fmt.Printf("Raw: %d chars (~%d tokens)\n", len(raw), rawToks)
		if rawExit != 0 {
			fmt.Printf("exit %d\n", rawExit)
		}
		return nil
	}

	filtered := res.Stdout
	if filtered == "" {
		filtered = "(empty — no output)\n"
	}
	filtToks := estimateToks(filtered)
	saved := rawToks - filtToks
	pct := 0
	if rawToks > 0 {
		pct = 100 * saved / rawToks
	}

	w := 62
	hr := strings.Repeat("─", w)
	thick := strings.Repeat("═", w)

	proveRow := func(s string) {
		vl := len(ansiRe.ReplaceAllString(s, ""))
		pad := w - vl
		if pad < 0 {
			pad = 0
		}
		fmt.Printf("║%s%s║\n", s, strings.Repeat(" ", pad))
	}

	fmt.Printf("╔%s╗\n", thick)
	title := fmt.Sprintf("  tok prove: %s %s", cmd, strings.Join(args, " "))
	proveRow(title)
	fmt.Printf("╠%s╣\n", thick)
	proveRow(fmt.Sprintf("  RAW    \033[2m%d tokens  (%d chars)\033[0m", rawToks, len(raw)))
	var filtLabel string
	if pct > 0 {
		filtLabel = fmt.Sprintf("  tok    \033[32m\033[1m%d tokens  (%d chars)  -%d%% saved\033[0m", filtToks, len(filtered), pct)
	} else if pct < 0 {
		filtLabel = fmt.Sprintf("  tok    \033[33m%d tokens  (%d chars)  +%d%% larger (no filter savings)\033[0m", filtToks, len(filtered), -pct)
	} else {
		filtLabel = fmt.Sprintf("  tok    %d tokens  (%d chars)  (unchanged)", filtToks, len(filtered))
	}
	proveRow(filtLabel)
	fmt.Printf("╠%s╣\n", thick)

	fmt.Printf("║  \033[2mRAW OUTPUT\033[0m%s║\n", strings.Repeat(" ", w-12))
	fmt.Printf("╟%s╢\n", hr)
	printProveLines(raw, w)
	fmt.Printf("╠%s╣\n", thick)

	fmt.Printf("║  \033[1mtok OUTPUT\033[0m%s║\n", strings.Repeat(" ", w-12))
	fmt.Printf("╟%s╢\n", hr)
	printProveLines(filtered, w)
	fmt.Printf("╚%s╝\n", thick)

	return nil
}

func printProveLines(text string, width int) {
	const maxLines = 20
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	shown := lines
	extra := 0
	if len(lines) > maxLines {
		shown = lines[:maxLines]
		extra = len(lines) - maxLines
	}
	inner := width - 4 // "║  " + content + "║"
	for _, l := range shown {
		if len(l) > inner {
			l = l[:inner-3] + "..."
		}
		fmt.Printf("║  %-*s║\n", inner, l)
	}
	if extra > 0 {
		msg := fmt.Sprintf("  … %d more lines omitted", extra)
		fmt.Printf("║\033[2m%-*s\033[0m║\n", width, msg)
	}
}

// estimateToks estimates token count using cl100k_base avg of ~3.5 chars/token.
func estimateToks(s string) int {
	return (len(s)*2 + 6) / 7
}
