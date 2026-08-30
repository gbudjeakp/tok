package filter

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	register(matchCloud, filterCloud)
}

func matchCloud(cmd, arg0 string) bool {
	switch cmd {
	case "aws":
		return true
	case "pulumi":
		return arg0 == "preview" || arg0 == "up" || arg0 == "destroy" || arg0 == "refresh" || arg0 == "stack"
	case "terraform", "tofu":
		return arg0 == "plan" || arg0 == "apply" || arg0 == "destroy" || arg0 == "show"
	case "gh":
		return arg0 == "pr" || arg0 == "issue" || arg0 == "run" || arg0 == "workflow"
	}
	return false
}

func filterCloud(cmd string, args []string) (*Result, error) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	sub2 := ""
	if len(args) > 1 {
		sub2 = args[1]
	}

	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)
	combined := stdout + stderr

	var out, id string

	switch {
	case cmd == "aws" && sub == "sts":
		out, id = awsSts(combined), "aws-sts"
	case cmd == "aws" && sub == "logs":
		out, id = deduplicateLogs(combined), "aws-logs"
	case cmd == "aws" && (sub == "ec2" || sub == "lambda" || sub == "iam"):
		out, id = awsCompact(combined), "aws-"+sub
	case cmd == "aws":
		out, id = awsGeneric(combined), "aws-generic"
	case cmd == "pulumi" && (sub == "preview" || sub == "up" || sub == "destroy"):
		out, id = pulumiOp(combined), "pulumi-" + sub
	case cmd == "terraform" || cmd == "tofu":
		out, id = terraformPlan(combined), "tf-"+sub
	case cmd == "gh" && sub == "pr" && sub2 == "list":
		out, id = ghPrList(combined), "gh-pr-list"
	case cmd == "gh" && sub == "run" && sub2 == "list":
		out, id = ghRunList(combined), "gh-run-list"
	default:
		// Try generic JSON compression as fallback for unrecognized subcommands
		if crushed, ok := SmashJSON(combined); ok {
			return &Result{Stdout: crushed, ExitCode: ec, OrigLen: orig, OutLen: len(crushed), FilterID: "json"}, nil
		}
		return &Result{Stdout: stdout, Stderr: stderr, ExitCode: ec, OrigLen: orig, OutLen: orig, FilterID: "cloud-pass"}, nil
	}

	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: id}, nil
}

// awsSts: single line identity summary.
var reArn = regexp.MustCompile(`"Arn":\s*"([^"]+)"`)
var reUserId = regexp.MustCompile(`"UserId":\s*"([^"]+)"`)
var reAccount = regexp.MustCompile(`"Account":\s*"([^"]+)"`)

func awsSts(output string) string {
	arn, userId, account := "", "", ""
	if m := reArn.FindStringSubmatch(output); m != nil {
		arn = m[1]
	}
	if m := reUserId.FindStringSubmatch(output); m != nil {
		userId = m[1]
	}
	if m := reAccount.FindStringSubmatch(output); m != nil {
		account = m[1]
	}
	if arn != "" {
		return fmt.Sprintf("Account: %s  User: %s  ARN: %s\n", account, userId, arn)
	}
	return output
}

// awsCompact: strip type annotations from DynamoDB/JSON output, keep essential fields.
var reAwsType = regexp.MustCompile(`"[SBNL]": `)

func awsCompact(output string) string {
	if len(output) < 500 {
		return output
	}
	// Strip JSON type wrappers: {"S": "value"} → "value"
	out := reAwsType.ReplaceAllString(output, `"value": `)
	// Keep only lines with actual values (not structural brackets with only whitespace)
	lines := strings.Split(out, "\n")
	var keep []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "{" || t == "}" || t == "[" || t == "]" || t == "," {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) == 0 {
		return output
	}
	return strings.Join(keep, "\n") + "\n"
}

func awsGeneric(output string) string {
	// Try JSON compression first (aws commands frequently return JSON)
	if crushed, ok := SmashJSON(strings.TrimSpace(output)); ok {
		return crushed
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 20 {
		return output
	}
	return heuristicSummary(output)
}

// pulumiOp: strip header URL and duration lines, keep resource changes.
var rePulumiNoise = regexp.MustCompile(`(?i)(Updating:|View Live:|Duration:|Permalink:|warning: pulumi)`)
var rePulumiChange = regexp.MustCompile(`(?i)(^\s+[+~\-] |error:|failed)`)

func pulumiOp(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var keep []string
	for _, l := range lines {
		if rePulumiNoise.MatchString(l) {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) == 0 {
		return output
	}
	return strings.Join(keep, "\n") + "\n"
}

// terraformPlan: keep only resource change lines and summary.
var reTfChange = regexp.MustCompile(`(?i)(^\s+[#+\-~] |Plan:|Apply complete|Error:|Changes to Outputs)`)
var reTfNoise = regexp.MustCompile(`(?i)(Refreshing state|Reading|Terraform will|terraform init|Acquiring state lock)`)

func terraformPlan(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var keep []string
	for _, l := range lines {
		if reTfNoise.MatchString(l) {
			continue
		}
		if reTfChange.MatchString(l) || strings.TrimSpace(l) == "" {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return output
	}
	return strings.Join(keep, "\n") + "\n"
}

// ghPrList: compact PR table.
var reGhPrLine = regexp.MustCompile(`^(\d+)\s+(.+?)\s{2,}(\S+)\s+(\S+)\s*$`)

func ghPrList(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var sb strings.Builder
	for _, l := range lines {
		if m := reGhPrLine.FindStringSubmatch(l); m != nil {
			sb.WriteString(fmt.Sprintf("#%-5s %-40s %-10s %s\n", m[1], truncate(m[2], 40), m[3], m[4]))
		} else {
			sb.WriteString(l + "\n")
		}
	}
	return sb.String()
}

func ghRunList(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 10 {
		return output
	}
	var keep []string
	reStatus := regexp.MustCompile(`(?i)(completed|failed|in_progress|queued|success|failure)`)
	for _, l := range lines {
		if reStatus.MatchString(l) || strings.TrimSpace(l) == "" {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return output
	}
	return strings.Join(keep, "\n") + "\n"
}
