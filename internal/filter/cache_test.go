package filter

import (
	"strings"
	"testing"
	"time"
)

func TestCheckCache_FirstCallEmpty(t *testing.T) {
	// Clear the cache to avoid cross-test interference.
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	got := checkCache("git", []string{"status"}, "output text")
	if got != "" {
		t.Fatalf("first call should return empty string, got %q", got)
	}
}

func TestCheckCache_SameOutputWithinTTL(t *testing.T) {
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	output := "some command output"
	checkCache("ls", []string{"-la"}, output)
	got := checkCache("ls", []string{"-la"}, output)

	if !strings.HasPrefix(got, "[unchanged:") {
		t.Fatalf("repeated identical output within TTL should return unchanged marker, got %q", got)
	}
}

func TestCheckCache_DifferentOutputNoMark(t *testing.T) {
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	checkCache("git", []string{"diff"}, "output A")
	got := checkCache("git", []string{"diff"}, "output B")

	if got != "" {
		t.Fatalf("different output should return empty string, got %q", got)
	}
}

func TestCheckCache_ExpiredTTL(t *testing.T) {
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	output := "stable output"
	checkCache("git", []string{"log"}, output)

	// Manually backdate the cache entry past the git TTL (30s).
	key := cacheKey("git", []string{"log"})
	cacheMu.Lock()
	e := cmdMap[key]
	e.when = time.Now().Add(-60 * time.Second)
	cmdMap[key] = e
	cacheMu.Unlock()

	got := checkCache("git", []string{"log"}, output)
	if got != "" {
		t.Fatalf("expired TTL should not return unchanged marker, got %q", got)
	}
}

func TestTTLFor_KnownCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		want time.Duration
	}{
		{"git", 30 * time.Second},
		{"docker", 60 * time.Second},
		{"aws", 60 * time.Second},
		{"ls", 30 * time.Second},
	}
	for _, c := range cases {
		if got := ttlFor(c.cmd); got != c.want {
			t.Errorf("ttlFor(%q): want %v, got %v", c.cmd, c.want, got)
		}
	}
}

func TestTTLFor_DefaultUnknown(t *testing.T) {
	if got := ttlFor("somethingobscure"); got != defaultTTL {
		t.Fatalf("unknown command should use defaultTTL, got %v", got)
	}
}
