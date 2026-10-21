package filter

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gbudjeakp/tok/internal/analytics"
)

// cmdCache caches recent command outputs keyed by "cmd args..." → (hash, time, charCount).
// If the same command produces the same output within its TTL, we return a
// "[unchanged: same output as Xs ago]" one-liner instead of the full text.
//
// tok runs as a fresh process per shell command (the hook spawns a new
// "tok <cmd>" process each time), so this cache is persisted to disk and
// reloaded on every invocation — an in-memory-only map would never hit.
type cacheEntry struct {
	Hash  string    `json:"h"` // hex-encoded sha256
	When  time.Time `json:"w"`
	Chars int       `json:"c"`
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

// checkCache returns a replacement string if the output is unchanged since the
// last cached run, or "" if the output should be shown normally.
// It always records the new output into the cache.
func checkCache(cmd string, args []string, output string) string {
	loadCacheOnce()

	key := cacheKey(cmd, args)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(output)))
	now := time.Now()
	ttl := ttlFor(cmd)

	cacheMu.Lock()
	defer cacheMu.Unlock()

	pruneLocked()

	if prev, ok := cmdMap[key]; ok {
		if prev.Hash == hash && now.Sub(prev.When) < ttl {
			age := int(now.Sub(prev.When).Seconds())
			// Keep the original timestamp so the TTL counts from first sighting.
			saveCache()
			return fmt.Sprintf("[unchanged: same %d-char output as %ds ago]\n", prev.Chars, age)
		}
	}

	cmdMap[key] = cacheEntry{Hash: hash, When: now, Chars: len(output)}
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
