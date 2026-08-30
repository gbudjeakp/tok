package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/gbudjeakp/tok/internal/filter"
)

type benchCase struct {
	cmd  string
	args []string
}

// defaultSuite covers the commands most commonly seen in agent sessions.
var defaultSuite = []benchCase{
	{"git", []string{"log", "--oneline", "-20"}},
	{"git", []string{"status"}},
	{"git", []string{"diff", "HEAD~1", "--stat"}},
	{"ls", []string{"-la"}},
	{"find", []string{".", "-name", "*.go", "-maxdepth", "3"}},
	{"find", []string{".", "-name", "*.ts", "-maxdepth", "4"}},
}

func newBenchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bench",
		Short: "Benchmark tok savings vs raw output and other installed tools",
		Long: `tok bench runs a curated suite of common commands and compares raw
output token cost against tok-filtered output. If rtk is installed, it also
runs each command through rtk for a direct head-to-head comparison.

Results show estimated token counts using cl100k_base (~3.5 chars/token).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBench()
		},
	}
}

func runBench() error {
	hasRtk := commandExists("rtk")

	fmt.Println("tok bench — running suite…")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if hasRtk {
		fmt.Fprintln(w, "COMMAND\tRAW\tTOK\tSAVED\tRTK\tRTK SAVED")
	} else {
		fmt.Fprintln(w, "COMMAND\tRAW\tTOK\tSAVED")
	}

	sep := strings.Repeat("─", 72)
	fmt.Fprintln(w, sep)

	var totalRaw, totalTok, totalRtk, ran int

	for _, c := range defaultSuite {
		raw, exitCode := filter.RawRun(c.cmd, c.args)
		if raw == "" || exitCode != 0 {
			continue // skip unavailable or erroring commands
		}
		rawTok := estimateToks(raw)

		res := filter.FilterOnly(c.cmd, c.args)
		filtTok := rawTok
		if res != nil {
			filtTok = estimateToks(res.Stdout)
		}

		saved := pct(rawTok, filtTok)
		label := c.cmd + " " + strings.Join(c.args, " ")
		if len(label) > 32 {
			label = label[:29] + "..."
		}

		line := fmt.Sprintf("%s\t%d\t%d\t%s", label, rawTok, filtTok, fmtSaved(saved))

		if hasRtk {
			rtkTok := estimateToks(runThrough("rtk", c.cmd, c.args))
			rtkSaved := pct(rawTok, rtkTok)
			totalRtk += rtkTok
			line += fmt.Sprintf("\t%d\t%s", rtkTok, fmtSaved(rtkSaved))
		}

		fmt.Fprintln(w, line)
		totalRaw += rawTok
		totalTok += filtTok
		ran++
	}

	fmt.Fprintln(w, sep)

	overall := pct(totalRaw, totalTok)
	summary := fmt.Sprintf("TOTAL (%d cmds)\t%d\t%d\t%s", ran, totalRaw, totalTok, fmtSaved(overall))
	if hasRtk {
		rtkOverall := pct(totalRaw, totalRtk)
		summary += fmt.Sprintf("\t%d\t%s", totalRtk, fmtSaved(rtkOverall))
	}
	fmt.Fprintln(w, summary)
	w.Flush()

	fmt.Println()
	if hasRtk {
		fmt.Printf("tok saves %d%% overall vs rtk's %d%% on this suite.\n", pct(totalRaw, totalTok), pct(totalRaw, totalRtk))
	} else {
		fmt.Printf("Install rtk (go install github.com/rtk-ai/rtk@latest) to compare head-to-head.\n")
	}
	return nil
}

func pct(raw, filtered int) int {
	if raw == 0 {
		return 0
	}
	return 100 * (raw - filtered) / raw
}

func fmtSaved(p int) string {
	if p > 0 {
		return fmt.Sprintf("-%d%%", p)
	}
	if p < 0 {
		return fmt.Sprintf("+%d%%", -p)
	}
	return "0%"
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runThrough runs cmd args through an external tool (e.g. rtk) and returns its stdout.
func runThrough(tool, cmd string, args []string) string {
	all := append([]string{cmd}, args...)
	out, err := exec.Command(tool, all...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
