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

func newSessionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "session",
		Short: "Show tok adoption rate in recent VS Code Copilot sessions",
		Long: `tok session reads recent VS Code Copilot transcripts and reports what
fraction of terminal commands were handled by tok vs passed through raw.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSession()
		},
	}
	return c
}

type sessionStat struct {
	total  int
	tokked int
	missed map[string]int
}

func runSession() error {
	transcripts, err := copilotTranscripts()
	if err != nil || len(transcripts) == 0 {
		fmt.Println("No VS Code Copilot transcripts found.")
		fmt.Println("Run some agent sessions in VS Code first.")
		return nil
	}

	// Only look at transcripts from the last 7 days.
	cutoff := time.Now().AddDate(0, 0, -7).Unix()
	var recent []string
	for _, p := range transcripts {
		info, err := os.Stat(p)
		if err == nil && info.ModTime().Unix() > cutoff {
			recent = append(recent, p)
		}
	}
	if len(recent) == 0 {
		fmt.Println("No recent transcripts (last 7 days). Run some agent sessions first.")
		return nil
	}
	// Cap at 10 most recent.
	sort.Slice(recent, func(i, j int) bool {
		si, _ := os.Stat(recent[i])
		sj, _ := os.Stat(recent[j])
		if si == nil || sj == nil {
			return false
		}
		return si.ModTime().After(sj.ModTime())
	})
	if len(recent) > 10 {
		recent = recent[:10]
	}

	total, tokked := 0, 0
	missed := map[string]int{}

	for _, p := range recent {
		stat := parseCopilotTranscript(p)
		total += stat.total
		tokked += stat.tokked
		for k, v := range stat.missed {
			missed[k] += v
		}
	}

	fmt.Printf("Analysed %d sessions (last 7 days)\n\n", len(recent))

	if total == 0 {
		fmt.Println("No run_in_terminal calls found. The agent hasn't run any shell commands recently.")
		return nil
	}

	pct := 100 * tokked / total
	fmt.Printf("tok adoption:  %d%%  (%d / %d terminal commands filtered)\n\n", pct, tokked, total)

	switch {
	case pct == 0:
		fmt.Println("Hook not firing yet. Run 'tok init -g' and restart VS Code.")
	case pct < 40:
		fmt.Println("Low adoption — many commands bypassing tok. Is the hook installed?")
	case pct < 75:
		fmt.Println("Good — hook is active. Missing filters below cover the rest.")
	default:
		fmt.Println("Excellent — tok is intercepting most commands.")
	}

	if len(missed) > 0 {
		fmt.Println("\nCommands tok could filter but didn't (potential savings):")
		type kv struct {
			k string
			v int
		}
		var sorted []kv
		for k, v := range missed {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
		for _, item := range sorted[:min(8, len(sorted))] {
			est := estimateSavings(item.k)
			fmt.Printf("  %-20s  %3d calls  ~%s savings\n", item.k, item.v, est)
		}
	}
	return nil
}

// copilotTranscripts returns all VS Code Copilot transcript JSONL paths.
func copilotTranscripts() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	wsRoot := filepath.Join(home, "Library", "Application Support", "Code", "User", "workspaceStorage")
	var paths []string
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		return nil, err
	}
	for _, ws := range entries {
		if !ws.IsDir() {
			continue
		}
		transcriptDir := filepath.Join(wsRoot, ws.Name(), "GitHub.copilot-chat", "transcripts")
		files, err := os.ReadDir(transcriptDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
				paths = append(paths, filepath.Join(transcriptDir, f.Name()))
			}
		}
	}
	return paths, nil
}

// copilotEvent matches the VS Code Copilot transcript event shape.
type copilotEvent struct {
	Type string `json:"type"`
	Data struct {
		ToolName  string          `json:"toolName"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"data"`
}

type runInTerminalArgs struct {
	Command string `json:"command"`
}

// knownTokTargets are commands tok can currently filter.
var knownTokTargets = map[string]bool{
	"git": true, "go": true, "cargo": true, "npm": true, "yarn": true, "pnpm": true,
	"bun": true, "pytest": true, "python": true, "docker": true, "kubectl": true,
	"ls": true, "find": true, "grep": true, "rg": true, "ruff": true, "eslint": true,
	"tsc": true, "golangci-lint": true, "aws": true, "gh": true, "pulumi": true,
	"terraform": true, "tofu": true, "pip": true, "pip3": true, "brew": true, "bundle": true,
}

func parseCopilotTranscript(path string) sessionStat {
	stat := sessionStat{missed: map[string]int{}}
	f, err := os.Open(path)
	if err != nil {
		return stat
	}
	defer f.Close()

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
		// Strip leading "cd /some/path && " before the real command.
		cmd := stripCdPrefix(args.Command)
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}
		stat.total++
		if parts[0] == "tok" {
			stat.tokked++
		} else if knownTokTargets[parts[0]] {
			stat.missed[parts[0]]++
		}
	}
	return stat
}

// stripCdPrefix removes a leading "cd /path && " so we classify the real command.
func stripCdPrefix(cmd string) string {
	if idx := strings.Index(cmd, "&&"); idx != -1 {
		return strings.TrimSpace(cmd[idx+2:])
	}
	return cmd
}

