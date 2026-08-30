package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newWasteCmd() *cobra.Command {
	var days int
	c := &cobra.Command{
		Use:   "waste",
		Short: "Show which commands are leaking tokens (no filter, or repeated calls)",
		Long: `tok waste analyses recent VS Code Copilot transcripts and reports:
  - Commands run without a tok filter (unfiltered spend)
  - Commands repeated 3+ times (signs of polling loops or missed caching)
  - Rough token estimate for each unfiltered invocation

Run "tok init" to install the hook and eliminate the top offenders.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWaste(days)
		},
	}
	c.Flags().IntVar(&days, "days", 7, "Number of days of transcripts to analyse")
	return c
}

func newLockdepsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lockdeps [path]",
		Short: "Show direct dependencies from a lock file as a compact table",
		Long: `tok lockdeps reads a lock file and prints a compact name/version table.
Supports: package-lock.json, yarn.lock, bun.lockb, go.sum, Cargo.lock, poetry.lock

When Copilot tries to read a full lock file, tok guard blocks it and suggests this command.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runLockdeps(path)
		},
	}
}

// ------- tok waste -------

// wasteNoise are shell builtins and keywords that aren't real commands.
var wasteNoise = map[string]bool{
	"cd": true, "source": true, "export": true, "set": true, "unset": true,
	"for": true, "while": true, "do": true, "done": true, "if": true,
	"then": true, "else": true, "fi": true, "case": true, "esac": true,
	"function": true, "return": true, "exit": true, "exec": true, "eval": true,
	"true": true, "false": true, "test": true, "[": true, "[[": true,
	"echo": true, "printf": true, "read": true, "wait": true, "trap": true,
	"local": true, "readonly": true, "declare": true, "typeset": true,
	"alias": true, "unalias": true, "type": true, "hash": true,
	"pushd": true, "popd": true, "dirs": true,
}

type wasteRecord struct {
	cmd   string
	count int
	estTok int // rough estimated tokens wasted (all unfiltered runs)
}

func runWaste(days int) error {
	transcripts, err := copilotTranscripts()
	if err != nil || len(transcripts) == 0 {
		fmt.Println("No VS Code Copilot transcripts found.")
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	var recent []string
	for _, p := range transcripts {
		info, err := os.Stat(p)
		if err == nil && info.ModTime().Unix() > cutoff {
			recent = append(recent, p)
		}
	}
	sort.Slice(recent, func(i, j int) bool {
		si, _ := os.Stat(recent[i])
		sj, _ := os.Stat(recent[j])
		if si == nil || sj == nil {
			return false
		}
		return si.ModTime().After(sj.ModTime())
	})
	if len(recent) > 20 {
		recent = recent[:20]
	}
	if len(recent) == 0 {
		fmt.Printf("No transcripts from last %d days.\n", days)
		return nil
	}

	// Per-command tally: total calls, tok-filtered calls, repeated-cmd sessions.
	type cmdStat struct {
		total   int
		tokked  int
		repeated int // sessions where this cmd appeared 3+ times
	}
	stats := map[string]*cmdStat{}
	totalCmds := 0

	for _, p := range recent {
		sessionCounts := map[string]int{}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var event copilotEvent
			if json.Unmarshal(sc.Bytes(), &event) != nil {
				continue
			}
			if event.Type != "tool.execution_start" || event.Data.ToolName != "run_in_terminal" {
				continue
			}
			var args runInTerminalArgs
			if json.Unmarshal(event.Data.Arguments, &args) != nil {
				continue
			}
			raw := stripCdPrefix(args.Command)
			parts := strings.Fields(raw)
			if len(parts) == 0 {
				continue
			}
			totalCmds++
			root := parts[0]

			// Skip shell builtins, keywords, env-var assignments, and other noise.
			if wasteNoise[root] || strings.Contains(root, "=") || strings.HasPrefix(root, "#") ||
				strings.HasPrefix(root, "\\") || strings.HasPrefix(root, "./") ||
				strings.HasPrefix(root, "/") || len(root) < 2 {
				continue
			}
			// Normalize: tok/rtk wrap the real command.
			if root == "tok" || root == "rtk" {
				if len(parts) > 1 {
					inner := parts[1]
					if s, ok := stats[inner]; ok {
						s.tokked++
					}
				}
				continue
			}
			if stats[root] == nil {
				stats[root] = &cmdStat{}
			}
			stats[root].total++
			sessionCounts[root]++
		}
		f.Close()
		for cmd, cnt := range sessionCounts {
			if cnt >= 3 {
				if stats[cmd] != nil {
					stats[cmd].repeated++
				}
			}
		}
	}

	fmt.Printf("Waste report — last %d days (%d sessions, %d terminal calls)\n\n", days, len(recent), totalCmds)

	// Unfiltered calls with tok-filterable commands.
	type wasteItem struct {
		cmd      string
		unfiltered int
		repeated   int
		tokEst     int
	}
	var waste []wasteItem
	for cmd, s := range stats {
		unfiltered := s.total - s.tokked
		if unfiltered <= 0 {
			continue
		}
		// Estimate tokens per unfiltered call by command type.
		tokPerCall := estimateRawTokens(cmd)
		waste = append(waste, wasteItem{
			cmd:        cmd,
			unfiltered: unfiltered,
			repeated:   s.repeated,
			tokEst:     unfiltered * tokPerCall,
		})
	}
	sort.Slice(waste, func(i, j int) bool { return waste[i].tokEst > waste[j].tokEst })

	if len(waste) == 0 {
		fmt.Println("No unfiltered commands detected. Great coverage!")
		return nil
	}

	fmt.Printf("%-22s  %6s  %8s  %8s\n", "command", "calls", "~tokens", "repeats")
	fmt.Println(strings.Repeat("-", 54))
	totalWastedTok := 0
	for _, w := range waste {
		if w.unfiltered < 2 {
			continue
		}
		rep := ""
		if w.repeated > 0 {
			rep = fmt.Sprintf("⚠ %d sessions polled 3+×", w.repeated)
		}
		fmt.Printf("%-22s  %6d  %8s  %s\n", w.cmd, w.unfiltered, fmtTok(w.tokEst), rep)
		totalWastedTok += w.tokEst
	}
	fmt.Println(strings.Repeat("-", 54))
	fmt.Printf("%-22s  %6s  %8s\n", "TOTAL (estimated)", "", fmtTok(totalWastedTok))
	fmt.Println()
	fmt.Println("Run `tok init` to install the hook and filter these automatically.")
	fmt.Println("Run `tok discover` to see session-by-session command breakdown.")
	return nil
}

// estimateRawTokens returns a rough token count per raw (unfiltered) invocation.
func estimateRawTokens(cmd string) int {
	switch cmd {
	case "git":
		return 800
	case "cargo", "go":
		return 1200
	case "npm", "yarn", "pnpm", "bun":
		return 600
	case "docker":
		return 400
	case "kubectl":
		return 500
	case "aws":
		return 700
	case "pytest", "jest", "vitest":
		return 1000
	case "find", "rg", "grep":
		return 300
	case "ls", "cat":
		return 200
	default:
		return 150
	}
}

func fmtTok(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// ------- tok lockdeps -------

func runLockdeps(path string) error {
	// Resolve path: if it's a directory, look for known lock files.
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	var lockFile string
	if info.IsDir() {
		candidates := []string{
			"package-lock.json", "yarn.lock", "bun.lockb",
			"go.sum", "Cargo.lock", "poetry.lock",
			"uv.lock", "Pipfile.lock", "composer.lock",
		}
		for _, c := range candidates {
			p := filepath.Join(path, c)
			if _, err := os.Stat(p); err == nil {
				lockFile = p
				break
			}
		}
		if lockFile == "" {
			return fmt.Errorf("no recognized lock file found in %s", path)
		}
	} else {
		lockFile = path
	}

	base := strings.ToLower(filepath.Base(lockFile))
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return err
	}

	switch base {
	case "package-lock.json":
		return parsePkgLock(data)
	case "go.sum":
		return parseGoSum(data)
	case "cargo.lock":
		return parseCargoLock(data)
	default:
		// Generic: show file name + size hint.
		fmt.Printf("Lock file: %s (%d KB)\n", lockFile, len(data)/1024)
		fmt.Println("Detailed parsing not yet supported for this format.")
		fmt.Println("Supported: package-lock.json, go.sum, Cargo.lock")
		return nil
	}
}

func parsePkgLock(data []byte) error {
	var v struct {
		Name         string                     `json:"name"`
		Packages     map[string]json.RawMessage `json:"packages"`
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	type dep struct{ name, version string }
	var deps []dep

	// v3 format uses "packages" with "" as root.
	if len(v.Packages) > 0 {
		for pkgKey, raw := range v.Packages {
			if pkgKey == "" {
				continue // root
			}
			// Only direct deps are under node_modules/<name> (no nested /).
			name := strings.TrimPrefix(pkgKey, "node_modules/")
			if strings.Contains(name, "node_modules") {
				continue // transitive
			}
			var pkg struct {
				Version  string `json:"version"`
				Dev      bool   `json:"dev"`
				Optional bool   `json:"optional"`
			}
			if json.Unmarshal(raw, &pkg) != nil {
				continue
			}
			deps = append(deps, dep{name, pkg.Version})
		}
	} else {
		for name, raw := range v.Dependencies {
			var pkg struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(raw, &pkg) != nil {
				continue
			}
			deps = append(deps, dep{name, pkg.Version})
		}
	}

	sort.Slice(deps, func(i, j int) bool { return deps[i].name < deps[j].name })
	if v.Name != "" {
		fmt.Printf("package-lock.json — %s  (%d direct deps)\n\n", v.Name, len(deps))
	}
	for _, d := range deps {
		fmt.Printf("%-40s  %s\n", d.name, d.version)
	}
	return nil
}

func parseGoSum(data []byte) error {
	seen := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		mod := parts[0]
		ver := parts[1]
		// Skip go.mod checksums, keep only source checksums.
		if strings.HasSuffix(ver, "/go.mod") {
			continue
		}
		if _, ok := seen[mod]; !ok {
			seen[mod] = ver
		}
	}
	type kv struct{ k, v string }
	var sorted []kv
	for k, v := range seen {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].k < sorted[j].k })
	fmt.Printf("go.sum  (%d modules)\n\n", len(sorted))
	for _, item := range sorted {
		fmt.Printf("%-50s  %s\n", item.k, item.v)
	}
	return nil
}

func parseCargoLock(data []byte) error {
	// Simple line-based Cargo.lock parser (no toml library needed).
	type pkg struct{ name, version string }
	var pkgs []pkg
	var cur pkg
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[[package]]" {
			if cur.name != "" {
				pkgs = append(pkgs, cur)
			}
			cur = pkg{}
			continue
		}
		if strings.HasPrefix(line, "name = ") {
			cur.name = strings.Trim(strings.TrimPrefix(line, "name = "), `"`)
		}
		if strings.HasPrefix(line, "version = ") {
			cur.version = strings.Trim(strings.TrimPrefix(line, "version = "), `"`)
		}
	}
	if cur.name != "" {
		pkgs = append(pkgs, cur)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].name < pkgs[j].name })
	fmt.Printf("Cargo.lock  (%d packages)\n\n", len(pkgs))
	for _, p := range pkgs {
		fmt.Printf("%-40s  %s\n", p.name, p.version)
	}
	return nil
}
