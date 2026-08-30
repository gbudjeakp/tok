package filter

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// cmdCache caches recent command outputs keyed by "cmd args..." → (hash, time, charCount).
// If the same command produces the same output within its TTL, we return a
// "[unchanged: same output as Xs ago]" one-liner instead of the full text.
type cacheEntry struct {
	hash  [32]byte
	when  time.Time
	chars int
}

var (
	cacheMu sync.Mutex
	cmdMap  = map[string]cacheEntry{}
)

// commandTTLs are per-command TTLs (seconds). Fast-changing commands get short TTLs.
var commandTTLs = map[string]time.Duration{
	"git":    30 * time.Second,
	"ls":     30 * time.Second,
	"find":   30 * time.Second,
	"docker": 60 * time.Second,
	"kubectl": 60 * time.Second,
	"aws":    60 * time.Second,
	"gh":     30 * time.Second,
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

// checkCache returns a replacement string if the output is unchanged since the
// last cached run, or "" if the output should be shown normally.
// It always records the new output into the cache.
func checkCache(cmd string, args []string, output string) string {
	key := cacheKey(cmd, args)
	hash := sha256.Sum256([]byte(output))
	now := time.Now()
	ttl := ttlFor(cmd)

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if prev, ok := cmdMap[key]; ok {
		if prev.hash == hash && now.Sub(prev.when) < ttl {
			age := int(now.Sub(prev.when).Seconds())
			cacheMu.Unlock()
			// Update timestamp so repeated hits keep the TTL fresh.
			cacheMu.Lock()
			cmdMap[key] = cacheEntry{hash: hash, when: prev.when, chars: prev.chars}
			return fmt.Sprintf("[unchanged: same %d-char output as %ds ago]\n", prev.chars, age)
		}
	}

	cmdMap[key] = cacheEntry{hash: hash, when: now, chars: len(output)}
	return ""
}

// pruneCache evicts entries older than their TTL. Called occasionally to avoid
// unbounded growth in long sessions.
func pruneCache() {
	now := time.Now()
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for k, v := range cmdMap {
		cmd := k
		// Extract base command (up to first \x00)
		for i, c := range k {
			if c == 0 {
				cmd = k[:i]
				break
			}
		}
		if now.Sub(v.when) > ttlFor(cmd)*2 {
			delete(cmdMap, k)
		}
	}
}
