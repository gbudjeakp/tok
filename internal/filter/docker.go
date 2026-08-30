package filter

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	register(func(cmd, _ string) bool {
		switch cmd {
		case "docker", "kubectl", "oc":
			return true
		}
		return false
	}, filterContainer)
}

func filterContainer(cmd string, args []string) (*Result, error) {
	stdout, stderr, ec := run(cmd, args)
	orig := len(stdout) + len(stderr)

	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	sub2 := ""
	if len(args) > 1 {
		sub2 = args[1]
	}

	var out, id string
	switch {
	case cmd == "docker" && sub == "ps":
		out, id = dockerPs(stdout), "docker-ps"
	case cmd == "docker" && sub == "images":
		out, id = dockerImages(stdout), "docker-images"
	case cmd == "docker" && sub == "logs":
		out, id = deduplicateLogs(stdout), "docker-logs"
	case cmd == "docker" && sub == "compose" && sub2 == "ps":
		out, id = dockerPs(stdout), "compose-ps"
	case (cmd == "kubectl" || cmd == "oc") && sub == "get" && sub2 == "pods":
		out, id = kubectlPods(stdout), "kubectl-pods"
	case (cmd == "kubectl" || cmd == "oc") && sub == "logs":
		out, id = deduplicateLogs(stdout+stderr), "kubectl-logs"
	default:
		return &Result{Stdout: stdout, Stderr: stderr, ExitCode: ec, OrigLen: orig, OutLen: orig, FilterID: "container-pass"}, nil
	}

	return &Result{Stdout: out, ExitCode: ec, OrigLen: orig, OutLen: len(out), FilterID: id}, nil
}

// dockerPs keeps: short ID, image, status, ports — drops COMMAND, CREATED, NAMES (shown at end)
func dockerPs(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return output
	}
	var sb strings.Builder
	// Parse tab/space-separated columns
	for i, l := range lines {
		if i == 0 {
			sb.WriteString("ID       IMAGE           STATUS    PORTS\n")
			continue
		}
		fields := splitFields(l)
		if len(fields) < 5 {
			sb.WriteString(l + "\n")
			continue
		}
		id := fields[0][:min(12, len(fields[0]))]
		image := truncate(fields[1], 20)
		status := truncate(fields[4], 20)
		ports := ""
		if len(fields) > 5 {
			ports = truncate(fields[5], 30)
		}
		sb.WriteString(fmt.Sprintf("%-12s %-20s %-20s %s\n", id, image, status, ports))
	}
	return sb.String()
}

func dockerImages(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var sb strings.Builder
	for i, l := range lines {
		if i == 0 {
			sb.WriteString("REPOSITORY           TAG      SIZE\n")
			continue
		}
		fields := splitFields(l)
		if len(fields) < 5 {
			sb.WriteString(l + "\n")
			continue
		}
		repo := truncate(fields[0], 20)
		tag := truncate(fields[1], 8)
		size := fields[4]
		sb.WriteString(fmt.Sprintf("%-20s %-8s %s\n", repo, tag, size))
	}
	return sb.String()
}

var reLogTS = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`)

func deduplicateLogs(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	seen := map[string]int{}
	var order []string
	for _, l := range lines {
		// Strip timestamp for dedup key
		key := reLogTS.ReplaceAllString(l, "")
		key = strings.TrimSpace(key)
		if count, ok := seen[key]; ok {
			seen[key] = count + 1
		} else {
			seen[key] = 1
			order = append(order, l)
		}
	}
	var sb strings.Builder
	for _, l := range order {
		key := reLogTS.ReplaceAllString(l, "")
		key = strings.TrimSpace(key)
		if n := seen[key]; n > 1 {
			sb.WriteString(fmt.Sprintf("%s  (×%d)\n", l, n))
		} else {
			sb.WriteString(l + "\n")
		}
	}
	return sb.String()
}

func kubectlPods(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var sb strings.Builder
	for _, l := range lines {
		fields := splitFields(l)
		if len(fields) < 3 {
			sb.WriteString(l + "\n")
			continue
		}
		// NAME  READY  STATUS  RESTARTS  AGE → NAME  STATUS  AGE
		name := truncate(fields[0], 32)
		status := fields[2]
		age := ""
		if len(fields) > 4 {
			age = fields[4]
		}
		sb.WriteString(fmt.Sprintf("%-32s %-12s %s\n", name, status, age))
	}
	return sb.String()
}

func splitFields(s string) []string {
	re := regexp.MustCompile(`\s{2,}|\t`)
	return re.Split(strings.TrimSpace(s), -1)
}
