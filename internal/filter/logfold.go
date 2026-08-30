package filter

import (
	"fmt"
	"strings"
)

// logfold collapses consecutive identical lines in s to "[Nx] line" markers.
// Lines that differ only in timestamps, request IDs, and counters are NOT
// collapsed (that's a harder problem); we only collapse exact duplicates.
// Applied to all filter output as a free post-processing step.
func logfold(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	// Preserve trailing newline state.
	trailingNL := strings.HasSuffix(s, "\n")
	if trailingNL && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= 3 {
		return s // not worth scanning tiny outputs
	}

	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		count := j - i
		if count >= 3 {
			out = append(out, fmt.Sprintf("[%dx] %s", count, lines[i]))
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}

	result := strings.Join(out, "\n")
	if trailingNL {
		result += "\n"
	}
	return result
}
