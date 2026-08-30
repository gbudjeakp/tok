package analytics

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	green  = "\033[32m"
	cyan   = "\033[36m"
	yellow = "\033[33m"
	dim    = "\033[2m"
)

// Show prints the savings dashboard to stdout.
func Show(history bool) {
	entries, err := Load()
	if err != nil || len(entries) == 0 {
		fmt.Println("No savings recorded yet. Run some commands with tok first.")
		fmt.Println("  tok git status")
		fmt.Println("  tok go test ./...")
		fmt.Println("  tok docker ps")
		return
	}

	var totalOrig, totalOut int
	counts := map[string]int{}
	origByFilter := map[string]int{}
	outByFilter := map[string]int{}
	now := time.Now().Unix()
	const month = int64(30 * 24 * 3600)

	for _, e := range entries {
		if now-e.TS > month {
			continue
		}
		totalOrig += e.OrigB
		totalOut += e.OutB
		counts[e.FilterID]++
		origByFilter[e.FilterID] += e.OrigB
		outByFilter[e.FilterID] += e.OutB
	}

	saved := totalOrig - totalOut
	toksSaved := saved / 4 // ~4 bytes per token
	pct := 0
	if totalOrig > 0 {
		pct = 100 * saved / totalOrig
	}
	usd := float64(toksSaved) / 1_000_000 * 15.0 // ~$15/Mtok blended

	width := 56
	bar := func() { fmt.Printf("╠%s╣\n", pad("═", width)) }
	top := func() { fmt.Printf("╔%s╗\n", pad("═", width)) }
	bot := func() { fmt.Printf("╚%s╝\n", pad("═", width)) }
	// row pads by visible character count so ANSI codes don't break alignment.
	row := func(s string) {
		vis := visLen(s)
		padding := width - 2 - vis
		if padding < 0 {
			padding = 0
		}
		fmt.Printf("║  %s%s║\n", s, strings.Repeat(" ", padding))
	}

	top()
	row(bold + "tok — Token Savings (last 30 days)" + reset)
	bar()
	row(fmt.Sprintf("%sSaved%s  ~%s%s%s tokens  (~$%.2f blended)", bold, reset, green, formatInt(toksSaved), reset, usd))
	row(fmt.Sprintf("Reduction  %s%d%%%s across %s commands", green, pct, reset, formatInt(len(entries))))
	bar()
	row(bold + "Top savers:" + reset)

	type kv struct {
		k string
		s int
	}
	var sorted []kv
	for k := range origByFilter {
		o, b := origByFilter[k], outByFilter[k]
		sorted = append(sorted, kv{k, o - b})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].s > sorted[j].s })
	if len(sorted) > 6 {
		sorted = sorted[:6]
	}
	for _, kv := range sorted {
		o := origByFilter[kv.k]
		p := 0
		if o > 0 {
			p = 100 * kv.s / o
		}
		bar := progressBar(p, 14)
		row(fmt.Sprintf("  %-18s %s%3d%%%s  %s %s", kv.k, green, p, reset, bar, dim+formatInt(kv.s/4)+" tok"+reset))
	}

	if history {
		bar()
		row(bold + "Recent commands:" + reset)
		start := len(entries) - 10
		if start < 0 {
			start = 0
		}
		for _, e := range entries[start:] {
			savedB := e.OrigB - e.OutB
			p := 0
			if e.OrigB > 0 {
				p = 100 * savedB / e.OrigB
			}
			t := time.Unix(e.TS, 0).Format("01/02 15:04")
			row(fmt.Sprintf("  %s  %-18s -%d%%", dim+t+reset, e.Cmd, p))
		}
	}

	bot()
}

func pad(ch string, n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += ch
	}
	return s
}

func formatInt(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	bar := green
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += dim + "░" + reset + dim
		}
	}
	return bar + reset
}

var ansiRe = regexp.MustCompile(`\033\[[0-9;]*m`)

// visLen returns the printable character count, ignoring ANSI escape sequences.
func visLen(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}
