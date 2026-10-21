package filter

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gbudjeakp/tok/internal/analytics"
)

// cmdCache caches recent command outputs keyed by "cmd args..." → (hash, time, charCount).
// If the same command produces the same output within its TTL, we return a
// "[unchanged: same output as Xs ago]" one-liner instead of the full text.
// If the output is merely *similar* (e.g. one container restarted in a
// `docker ps` list), we return only the new lines plus a change summary —
// the agent already has the unchanged lines in its own conversation history
// from the earlier call. This is a lexical stand-in for the semantic/fuzzy
// caching idea popularized by GPTCache, without needing embeddings.
//
// tok runs as a fresh process per shell command (the hook spawns a new
// "tok <cmd>" process each time), so this cache is persisted to disk and
// reloaded on every invocation — an in-memory-only map would never hit.
type cacheEntry struct {
	Hash  string    `json:"h"` // hex-encoded sha256 of the full output
	When  time.Time `json:"w"`
	Chars int       `json:"c"`
	Lines []uint32  `json:"l,omitempty"` // per-line fnv32 hashes, for near-dup detection
}

var (
	cacheMu   sync.Mutex
	cmdMap    = map[string]cacheEntry{}
	cacheOnce sync.Once
)

// commandTTLs are per-command TTLs (seconds). Fast-changing commands get short TTLs.
var commandTTLs = map[string]time.Duration{
	"git":     30 * time.Second,
	"ls":      30 * time.Second,
	"find":    30 * time.Second,
	"docker":  60 * time.Second,
	"kubectl": 60 * time.Second,
	"aws":     60 * time.Second,
	"gh":      30 * time.Second,
}

const defaultTTL = 5 * time.Minute

// maxCacheLines bounds how many line-hashes we keep per entry (cache stays
// small; tok's filtered output is designed to already be compact).
const maxCacheLines = 500

// nearDupThreshold: outputs sharing at least this fraction of lines (Jaccard
// similarity) are treated as "mostly the same" and collapsed to a diff.
const nearDupThreshold = 0.85

func ttlFor(cmd string) time.Duration {
	if d, ok := commandTTLs[cmd]; ok {
		return d
	}
	return defaultTTL
}

// cacheKey returns the map key for a command invocation.
func cacheKey(cmd string, args []string) string {
	key := cmd
	for _, a := range args {
		key += "\x00" + a
	}
	return key
}

func cacheFilePath() (string, error) {
	dir, err := analytics.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache.json"), nil
}

// loadCacheOnce reads the persisted cache from disk into cmdMap, once per process.
func loadCacheOnce() {
	cacheOnce.Do(func() {
		path, err := cacheFilePath()
		if err != nil {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var loaded map[string]cacheEntry
		if json.Unmarshal(data, &loaded) == nil {
			cmdMap = loaded
		}
	})
}

// saveCache writes cmdMap to disk atomically (temp file + rename) so
// concurrent tok invocations never observe a half-written file.
func saveCache() {
	path, err := cacheFilePath()
	if err != nil {
		return
	}
	data, err := json.Marshal(cmdMap)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// lineHashes splits s into lines and returns an fnv32 hash per line, capped
// at maxCacheLines (tok's filtered output is already compact, so this rarely
// truncates in practice).
func lineHashes(s string) ([]string, []uint32) {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > maxCacheLines {
		lines = lines[:maxCacheLines]
	}
	hashes := make([]uint32, len(lines))
	for i, l := range lines {
		h := fnv.New32a()
		_, _ = h.Write([]byte(l))
		hashes[i] = h.Sum32()
	}
	return lines, hashes
}

// jaccard returns the set similarity of two hash slices: |intersection| / |union|.
func jaccard(a, b []uint32) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	setA := make(map[uint32]bool, len(a))
	for _, v := range a {
		setA[v] = true
	}
	setB := make(map[uint32]bool, len(b))
	for _, v := range b {
		setB[v] = true
	}
	inter := 0
	for v := range setA {
		if setB[v] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

// checkCache returns a replacement string if the output is unchanged or
// near-duplicate since the last cached run, or "" if the output should be
// shown normally. It always records the new output into the cache.
func checkCache(cmd string, args []string, output string) string {
	loadCacheOnce()

	key := cacheKey(cmd, args)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(output)))
	curLines, curHashes := lineHashes(output)
	now := time.Now()
	ttl := ttlFor(cmd)

	cacheMu.Lock()
	defer cacheMu.Unlock()

	pruneLocked()

	prev, ok := cmdMap[key]
	fresh := ok && now.Sub(prev.When) < ttl

	if fresh && prev.Hash == hash {
		age := int(now.Sub(prev.When).Seconds())
		// Keep the original timestamp so the TTL counts from first sighting.
		saveCache()
		return fmt.Sprintf("[unchanged: same %d-char output as %ds ago]\n", prev.Chars, age)
	}

	if fresh && len(prev.Lines) > 0 && len(curHashes) > 0 {
		if sim := jaccard(prev.Lines, curHashes); sim >= nearDupThreshold {
			prevSet := make(map[uint32]bool, len(prev.Lines))
			for _, v := range prev.Lines {
				prevSet[v] = true
			}
			var added []string
			for i, h := range curHashes {
				if !prevSet[h] {
					added = append(added, curLines[i])
				}
			}
			removed := len(prev.Lines) - (len(curHashes) - len(added))
			cmdMap[key] = cacheEntry{Hash: hash, When: now, Chars: len(output), Lines: curHashes}
			saveCache()
			summary := fmt.Sprintf("[~%.0f%% same as %ds ago, +%d/-%d lines]\n", sim*100, int(now.Sub(prev.When).Seconds()), len(added), removed)
			if len(added) == 0 {
				return summary
			}
			return summary + strings.Join(added, "\n") + "\n"
		}
	}

	cmdMap[key] = cacheEntry{Hash: hash, When: now, Chars: len(output), Lines: curHashes}
	saveCache()
	return ""
}

// pruneLocked evicts entries older than 2x their TTL. Caller must hold cacheMu.
func pruneLocked() {
	now := time.Now()
	for k, v := range cmdMap {
		cmd := k
		// Extract base command (up to first \x00)
		for i, c := range k {
			if c == 0 {
				cmd = k[:i]
				break
			}
		}
		if now.Sub(v.When) > ttlFor(cmd)*2 {
			delete(cmdMap, k)
		}
	}
}

