package filter

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SmashJSON compresses JSON tool output. Returns the compressed string and true
// if the input was valid JSON worth compressing, otherwise ("", false).
func SmashJSON(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if len(s) == 0 || (s[0] != '{' && s[0] != '[') {
		return "", false
	}

	// Array of objects
	var arr []json.RawMessage
	if json.Unmarshal([]byte(s), &arr) == nil && len(arr) > 0 {
		return crushArray(arr), true
	}

	// Single object
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(s), &obj) == nil && len(obj) > 0 {
		return crushObject(obj, 0), true
	}

	return "", false
}

func crushArray(arr []json.RawMessage) string {
	// Detect if it's an array of objects (same schema = compact table)
	var rows []map[string]json.RawMessage
	for _, item := range arr {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(item, &m); err != nil {
			// Mixed array — just show count + first few items
			return mixedArray(arr)
		}
		rows = append(rows, m)
	}

	keys := unionKeys(rows)
	const maxRows = 12

	var sb strings.Builder
	fmt.Fprintf(&sb, "[%d items]\n", len(arr))
	fmt.Fprintf(&sb, "%s\n", strings.Join(keys, "  "))
	fmt.Fprintf(&sb, "%s\n", strings.Repeat("─", min(80, 4*len(keys))))

	shown := rows
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for _, row := range shown {
		vals := make([]string, len(keys))
		for i, k := range keys {
			v := row[k]
			vals[i] = flatVal(v)
		}
		sb.WriteString(strings.Join(vals, "  "))
		sb.WriteByte('\n')
	}
	if len(rows) > maxRows {
		fmt.Fprintf(&sb, "… %d more\n", len(rows)-maxRows)
	}
	return sb.String()
}

func crushObject(obj map[string]json.RawMessage, depth int) string {
	const maxDepth = 2
	const maxKeys = 20
	keys := sortedKeys(obj)
	if len(keys) > maxKeys {
		keys = keys[:maxKeys]
	}
	var sb strings.Builder
	indent := strings.Repeat("  ", depth)
	for _, k := range keys {
		v := obj[k]
		// Recurse into nested objects
		var nested map[string]json.RawMessage
		if depth < maxDepth && json.Unmarshal(v, &nested) == nil && len(nested) > 0 {
			fmt.Fprintf(&sb, "%s%s:\n%s", indent, k, crushObject(nested, depth+1))
			continue
		}
		// Recurse into nested arrays
		var arr []json.RawMessage
		if json.Unmarshal(v, &arr) == nil {
			if len(arr) == 0 {
				fmt.Fprintf(&sb, "%s%s: []\n", indent, k)
			} else if depth < maxDepth {
				fmt.Fprintf(&sb, "%s%s: %s", indent, k, crushArray(arr))
			} else {
				fmt.Fprintf(&sb, "%s%s: [%d items]\n", indent, k, len(arr))
			}
			continue
		}
		fmt.Fprintf(&sb, "%s%s: %s\n", indent, k, flatVal(v))
	}
	if len(obj) > maxKeys {
		fmt.Fprintf(&sb, "%s… %d more keys\n", indent, len(obj)-maxKeys)
	}
	return sb.String()
}

func mixedArray(arr []json.RawMessage) string {
	const maxShow = 5
	var sb strings.Builder
	fmt.Fprintf(&sb, "[%d items]\n", len(arr))
	for i, item := range arr {
		if i >= maxShow {
			fmt.Fprintf(&sb, "… %d more\n", len(arr)-maxShow)
			break
		}
		sb.WriteString(flatVal(item))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// flatVal renders a json.RawMessage as a short readable string.
func flatVal(v json.RawMessage) string {
	if v == nil {
		return "-"
	}
	s := strings.TrimSpace(string(v))
	if len(s) == 0 {
		return "-"
	}
	// Unwrap AWS-style type wrappers: {"S":"val"} → val, {"N":"42"} → 42, etc.
	var wrapper map[string]json.RawMessage
	if json.Unmarshal(v, &wrapper) == nil && len(wrapper) == 1 {
		for k, inner := range wrapper {
			if k == "S" || k == "N" || k == "BOOL" || k == "NULL" || k == "B" {
				return flatVal(inner)
			}
		}
	}
	// Strip outer quotes
	var str string
	if json.Unmarshal(v, &str) == nil {
		if len(str) > 40 {
			return str[:37] + "..."
		}
		return str
	}
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}

func unionKeys(rows []map[string]json.RawMessage) []string {
	seen := map[string]int{}
	for _, r := range rows {
		for k := range r {
			seen[k]++
		}
	}
	// Sort by frequency desc, then alphabetically
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if seen[keys[i]] != seen[keys[j]] {
			return seen[keys[i]] > seen[keys[j]]
		}
		return keys[i] < keys[j]
	})
	const maxCols = 8
	if len(keys) > maxCols {
		keys = keys[:maxCols]
	}
	return keys
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
