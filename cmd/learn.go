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
	"unicode/utf8"

	"github.com/spf13/cobra"
)

func newLearnCmd() *cobra.Command {
	var dryRun bool
	var days int
	c := &cobra.Command{
		Use:   "learn",
		Short: "Mine session transcripts for failure patterns and write lessons",
		Long: `tok learn scans recent VS Code Copilot session transcripts for command
failure patterns and repeated error sequences. It writes a concise summary of
lessons to CLAUDE.local.md in the current directory (or ~/.claude/CLAUDE.local.md
with --global). Agents that read CLAUDE.local.md will avoid re-hitting the same
errors.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			global, _ := cmd.Flags().GetBool("global")
			return runLearn(global, dryRun, days)
		},
	}
	c.Flags().Bool("global", false, "Write to ~/.claude/CLAUDE.local.md (global)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Print lessons without writing to file")
	c.Flags().IntVar(&days, "days", 7, "Number of days of transcripts to scan")
	return c
}

// learnEvent captures the fields we care about from a transcript event.
type learnEvent struct {
	typ     string // session.start, assistant.message, tool.execution_start, tool.execution_complete
	content string // assistant.message content
	toolID  string // tool call id
	toolName string
	cmdStr  string // run_in_terminal command
	success bool   // tool.execution_complete success flag
}

// failPattern is one detected failure pattern.
type failPattern struct {
	cmd      string // command that was repeated/failed
	context  string // brief description of what went wrong
	count    int    // sessions it appeared in
}

func runLearn(global, dryRun bool, days int) error {
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
	if len(recent) == 0 {
		fmt.Printf("No transcripts in last %d days. Try --days N.\n", days)
		return nil
	}
	fmt.Printf("Scanning %d sessions from last %d days…\n", len(recent), days)

	patterns := minePatterns(recent)
	if len(patterns) == 0 {
		fmt.Println("No repeated failure patterns found. ")
		return nil
	}

	lessons := formatLessons(patterns)
	if dryRun {
		fmt.Println("\n── Lessons (dry-run) ──────────────────────────")
		fmt.Println(lessons)
		return nil
	}

	outPath, err := learnOutputPath(global)
	if err != nil {
		return err
	}
	if err := writeLearnFile(outPath, lessons); err != nil {
		return err
	}

	fmt.Printf("\nWrote %d lesson(s) to %s\n", len(patterns), outPath)
	fmt.Println("Agents that include this file will avoid repeating these mistakes.")
	return nil
}

// ─── transcript mining ────────────────────────────────────────────────────────

type sessionEvents []learnEvent

// noisyRoots are commands that are legitimately repeated often and shouldn't
// be flagged as failure patterns.
var noisyRoots = map[string]bool{
	"cd": true, "ls": true, "cat": true, "echo": true, "cp": true, "mv": true,
	"mkdir": true, "rm": true, "touch": true, "source": true, "export": true,
	"#": true, "//": true, "true": true, "false": true, "test": true,
	"head": true, "tail": true, "which": true, "find": true, "wc": true,
	"sed": true, "awk": true, "sort": true, "uniq": true, "xargs": true,
}

func minePatterns(paths []string) []failPattern {
	// cmd → number of sessions where we saw it fail or be retried
	cmdFails := map[string][]string{} // cmd → contexts across sessions

	for _, p := range paths {
		events := readLearnEvents(p)
		sessionFails := detectSessionFailures(events)
		for cmd, contexts := range sessionFails {
			cmdFails[cmd] = append(cmdFails[cmd], contexts...)
		}
	}

	var patterns []failPattern
	for cmd, contexts := range cmdFails {
		if noisyRoots[cmd] {
			continue // skip frequent-but-benign commands
		}
		if len(contexts) < 2 {
			continue // only care about patterns that recur
		}
		// Deduplicate contexts
		seen := map[string]bool{}
		var unique []string
		for _, c := range contexts {
			key := c[:min8(c, 60)]
			if !seen[key] {
				seen[key] = true
				unique = append(unique, c)
			}
		}
		patterns = append(patterns, failPattern{
			cmd:     cmd,
			context: bestContext(unique),
			count:   len(contexts),
		})
	}

	// Sort by count descending
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].count > patterns[j].count
	})
	const maxPatterns = 8
	if len(patterns) > maxPatterns {
		patterns = patterns[:maxPatterns]
	}
	return patterns
}

// detectSessionFailures reads events from one session and returns a map of
// cmd → failure context strings.
func detectSessionFailures(events sessionEvents) map[string][]string {
	fails := map[string][]string{}

	// Build a callID → command map from tool execution starts
	callToCmd := map[string]string{}
	for _, e := range events {
		if e.typ == "tool.execution_start" && e.toolName == "run_in_terminal" && e.cmdStr != "" {
			callToCmd[e.toolID] = e.cmdStr
		}
	}

	// Track which commands were run and in what order
	var cmdSequence []string
	for _, e := range events {
		if e.typ == "tool.execution_start" && e.toolName == "run_in_terminal" {
			cmdSequence = append(cmdSequence, e.cmdStr)
		}
	}

	// Pattern 1: tool.execution_complete with success=false
	for _, e := range events {
		if e.typ == "tool.execution_complete" && !e.success {
			if cmd, ok := callToCmd[e.toolID]; ok {
				root := cmdRoot(cmd)
				fails[root] = append(fails[root], "tool execution failed: "+truncate(cmd, 80))
			}
		}
	}

	// Pattern 2: same root command repeated 3+ times in a session (agent struggling)
	cmdCount := map[string]int{}
	cmdExample := map[string]string{}
	for _, cmd := range cmdSequence {
		root := cmdRoot(cmd)
		cmdCount[root]++
		if _, ok := cmdExample[root]; !ok {
			cmdExample[root] = cmd
		}
	}
	for root, count := range cmdCount {
		if count >= 3 {
			fails[root] = append(fails[root],
				fmt.Sprintf("run %d times in one session: %s", count, truncate(cmdExample[root], 80)))
		}
	}

	// Pattern 3: assistant message contains error-acknowledgment followed by correction
	for i, e := range events {
		if e.typ != "assistant.message" {
			continue
		}
		content := strings.ToLower(e.content)
		if !containsErrorSignal(content) {
			continue
		}
		// Look ahead for the next run_in_terminal to infer the correction context
		for j := i + 1; j < len(events) && j < i+5; j++ {
			next := events[j]
			if next.typ == "tool.execution_start" && next.toolName == "run_in_terminal" {
				root := cmdRoot(next.cmdStr)
				snippet := extractErrorSentence(e.content)
				if snippet != "" {
					fails[root] = append(fails[root], snippet)
				}
				break
			}
		}
	}

	return fails
}

func containsErrorSignal(lower string) bool {
	signals := []string{
		"error:", "failed:", "traceback", "panic:", "exit status",
		"permission denied", "no such file", "command not found",
		"undefined:", "cannot find", "does not exist", "syntax error",
	}
	for _, s := range signals {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// extractErrorSentence pulls the first sentence mentioning an error keyword.
func extractErrorSentence(content string) string {
	sentences := strings.FieldsFunc(content, func(r rune) bool {
		return r == '.' || r == '\n'
	})
	for _, s := range sentences {
		lower := strings.ToLower(s)
		if containsErrorSignal(lower) {
			s = strings.TrimSpace(s)
			return truncate(s, 100)
		}
	}
	return ""
}

func cmdRoot(cmdStr string) string {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return "unknown"
	}
	// Strip path
	base := filepath.Base(fields[0])
	// Special: "npx something" → "npx something"
	if (base == "npx" || base == "sudo") && len(fields) > 1 {
		return base + " " + filepath.Base(fields[1])
	}
	return base
}

func bestContext(contexts []string) string {
	if len(contexts) == 0 {
		return ""
	}
	// Return the most informative one (longest, up to 120 chars)
	best := contexts[0]
	for _, c := range contexts[1:] {
		if len(c) > len(best) {
			best = c
		}
	}
	return truncate(best, 120)
}

// ─── output formatting ────────────────────────────────────────────────────────

func formatLessons(patterns []failPattern) string {
	var sb strings.Builder
	sb.WriteString("<!-- tok-learn: start — remove this block to disable -->\n")
	sb.WriteString("# Tok-Learn: Failure Patterns\n\n")
	sb.WriteString("Lessons mined from recent session transcripts. ")
	sb.WriteString("These patterns appeared repeatedly — avoid repeating them.\n\n")
	for i, p := range patterns {
		sb.WriteString(fmt.Sprintf("%d. **`%s`** (seen %d times)\n", i+1, p.cmd, p.count))
		if p.context != "" {
			sb.WriteString(fmt.Sprintf("   - %s\n", p.context))
		}
	}
	sb.WriteString("<!-- tok-learn: end -->\n")
	return sb.String()
}

func learnOutputPath(global bool) (string, error) {
	if !global {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, "CLAUDE.local.md"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "CLAUDE.local.md"), nil
}

func writeLearnFile(path, lessons string) error {
	// Read existing file if any
	existing, _ := os.ReadFile(path)

	const startMarker = "<!-- tok-learn: start"
	const endMarker = "<!-- tok-learn: end -->"

	content := string(existing)
	if start := strings.Index(content, startMarker); start >= 0 {
		end := strings.Index(content, endMarker)
		if end >= 0 {
			end += len(endMarker) + 1 // include trailing newline
			content = content[:start] + content[end:]
		}
	}
	// Append new lessons
	if !strings.HasSuffix(strings.TrimSpace(content), "") {
		content = strings.TrimRight(content, "\n") + "\n\n"
	} else {
		content = strings.TrimRight(content, "\n") + "\n\n"
	}
	content += lessons

	return os.WriteFile(path, []byte(content), 0o644)
}

// ─── transcript reader ────────────────────────────────────────────────────────

type rawLearnEvent struct {
	Type string `json:"type"`
	Data struct {
		MessageID  string          `json:"messageId"`
		Content    string          `json:"content"`
		ToolName   string          `json:"toolName"`
		ToolCallID string          `json:"toolCallId"`
		Arguments  json.RawMessage `json:"arguments"`
		Success    bool            `json:"success"`
	} `json:"data"`
}

func readLearnEvents(path string) sessionEvents {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var events sessionEvents
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB max line
	for scanner.Scan() {
		var raw rawLearnEvent
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		e := learnEvent{
			typ:      raw.Type,
			content:  raw.Data.Content,
			toolID:   raw.Data.ToolCallID,
			toolName: raw.Data.ToolName,
			success:  raw.Data.Success,
		}
		if raw.Type == "tool.execution_start" && raw.Data.ToolName == "run_in_terminal" {
			var args runInTerminalArgs
			if err := json.Unmarshal(raw.Data.Arguments, &args); err == nil {
				e.cmdStr = args.Command
			}
		}
		events = append(events, e)
	}
	return events
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-3]) + "..."
}

func min8(s string, n int) int {
	l := len(s)
	if l < n {
		return l
	}
	return n
}
