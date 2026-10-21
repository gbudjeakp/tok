package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/gbudjeakp/tok/internal/analytics"
)

func newBoardCmd() *cobra.Command {
	var noOpen bool
	c := &cobra.Command{
		Use:   "board",
		Short: "Generate a local HTML dashboard of savings over time",
		Long: `tok board renders a static HTML page from your local savings history
(~/.local/share/tok/history.ndjson): tokens saved per day, active-day streak,
and top commands by savings. Nothing leaves your machine.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoard(!noOpen)
		},
	}
	c.Flags().BoolVar(&noOpen, "no-open", false, "write the HTML file without opening a browser")
	return c
}

func runBoard(openBrowser bool) error {
	entries, err := analytics.Load()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No savings recorded yet. Run some commands with tok first.")
		return nil
	}

	dir, err := analytics.DataDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "board.html")

	html := renderBoard(entries)
	if err := writeFile(path, html); err != nil {
		return err
	}
	fmt.Println("Wrote", path)

	if openBrowser {
		openInBrowser(path)
	}
	return nil
}

func renderBoard(entries []analytics.Entry) string {
	const dayLayout = "2006-01-02"
	byDay := map[string]int{} // date -> tokens saved
	byFilter := map[string]int{}
	countByFilter := map[string]int{}
	var totalOrig, totalOut int
	days := map[string]bool{}

	for _, e := range entries {
		saved := e.OrigB - e.OutB
		t := time.Unix(e.TS, 0)
		key := t.Format(dayLayout)
		byDay[key] += saved / 4
		byFilter[e.FilterID] += saved
		countByFilter[e.FilterID]++
		totalOrig += e.OrigB
		totalOut += e.OutB
		days[key] = true
	}

	totalSavedToks := (totalOrig - totalOut) / 4
	usd := float64(totalSavedToks) / 1_000_000 * 15.0
	streak := currentStreak(days)

	// last 30 days of bars
	var bars []string
	maxVal := 1
	var dayKeys []string
	for i := 29; i >= 0; i-- {
		k := time.Now().AddDate(0, 0, -i).Format(dayLayout)
		dayKeys = append(dayKeys, k)
		if byDay[k] > maxVal {
			maxVal = byDay[k]
		}
	}
	for _, k := range dayKeys {
		v := byDay[k]
		h := 4 + (v*90)/maxVal
		bars = append(bars, fmt.Sprintf(
			`<div class="bar" style="height:%dpx" title="%s: ~%dk tok"></div>`, h, k, v/1000))
	}

	type kv struct {
		k string
		v int
	}
	var topFilters []kv
	for k, v := range byFilter {
		topFilters = append(topFilters, kv{k, v})
	}
	sort.Slice(topFilters, func(i, j int) bool { return topFilters[i].v > topFilters[j].v })
	if len(topFilters) > 10 {
		topFilters = topFilters[:10]
	}

	var rows strings.Builder
	for _, f := range topFilters {
		rows.WriteString(fmt.Sprintf(
			"<tr><td>%s</td><td>%d</td><td>~%dk tok</td></tr>\n",
			f.k, countByFilter[f.k], f.v/4000))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>tok board</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;background:#0d1117;color:#e6edf3;margin:0;padding:2rem;}
h1{font-size:1.3rem;margin-bottom:1.5rem;}
.cards{display:flex;gap:1rem;margin-bottom:2rem;flex-wrap:wrap;}
.card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:1rem 1.5rem;min-width:160px;}
.card .label{font-size:.75rem;color:#8b949e;text-transform:uppercase;letter-spacing:.05em;}
.card .value{font-size:1.6rem;font-weight:600;margin-top:.25rem;color:#3fb950;}
.chart{display:flex;align-items:flex-end;gap:3px;height:100px;background:#161b22;border:1px solid #30363d;border-radius:8px;padding:1rem;margin-bottom:2rem;}
.bar{flex:1;background:#3fb950;border-radius:2px 2px 0 0;min-width:4px;}
table{width:100%%;border-collapse:collapse;background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden;}
th,td{text-align:left;padding:.5rem 1rem;border-bottom:1px solid #30363d;font-size:.9rem;}
th{color:#8b949e;font-weight:500;}
</style></head>
<body>
<h1>tok board — local savings dashboard</h1>
<div class="cards">
  <div class="card"><div class="label">Tokens saved</div><div class="value">~%dk</div></div>
  <div class="card"><div class="label">Est. cost saved</div><div class="value">$%.2f</div></div>
  <div class="card"><div class="label">Commands filtered</div><div class="value">%d</div></div>
  <div class="card"><div class="label">Active-day streak</div><div class="value">%d</div></div>
</div>
<div class="chart">%s</div>
<table>
<tr><th>Command</th><th>Calls</th><th>Tokens saved</th></tr>
%s</table>
</body></html>
`, totalSavedToks/1000, usd, len(entries), streak, strings.Join(bars, ""), rows.String())
}

// currentStreak counts consecutive days (ending today) with at least one entry.
func currentStreak(days map[string]bool) int {
	streak := 0
	for i := 0; ; i++ {
		k := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		if !days[k] {
			break
		}
		streak++
	}
	return streak
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func openInBrowser(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}
