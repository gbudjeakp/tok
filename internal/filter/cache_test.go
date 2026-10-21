package filter

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCheckCache_FirstCallEmpty(t *testing.T) {
	t.Setenv("TOK_DATA_DIR", t.TempDir())
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
	t.Setenv("TOK_DATA_DIR", t.TempDir())
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
	t.Setenv("TOK_DATA_DIR", t.TempDir())
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
	t.Setenv("TOK_DATA_DIR", t.TempDir())
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	output := "stable output"
	checkCache("git", []string{"log"}, output)

	// Manually backdate the cache entry past the git TTL (30s).
	key := cacheKey("git", []string{"log"})
	cacheMu.Lock()
	e := cmdMap[key]
	e.When = time.Now().Add(-60 * time.Second)
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

func TestJaccard_Identical(t *testing.T) {
	a := []uint32{1, 2, 3}
	if got := jaccard(a, a); got != 1 {
		t.Fatalf("identical sets should have similarity 1, got %v", got)
	}
}

func TestJaccard_Disjoint(t *testing.T) {
	a := []uint32{1, 2, 3}
	b := []uint32{4, 5, 6}
	if got := jaccard(a, b); got != 0 {
		t.Fatalf("disjoint sets should have similarity 0, got %v", got)
	}
}

func TestJaccard_PartialOverlap(t *testing.T) {
	a := []uint32{1, 2, 3, 4}
	b := []uint32{1, 2, 5, 6}
	// intersection={1,2}=2, union={1,2,3,4,5,6}=6 -> 2/6
	want := 2.0 / 6.0
	if got := jaccard(a, b); got != want {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestCheckCache_NearDuplicateShowsDiff(t *testing.T) {
	t.Setenv("TOK_DATA_DIR", t.TempDir())
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	// 20 lines, only the last one changes between calls (jaccard ~0.90).
	base := make([]string, 20)
	for i := range base {
		base[i] = fmt.Sprintf("line%d", i)
	}
	first := strings.Join(base, "\n")
	checkCache("docker", []string{"ps"}, first)

	base[19] = "line19-changed"
	second := strings.Join(base, "\n")
	got := checkCache("docker", []string{"ps"}, second)

	if !strings.HasPrefix(got, "[~") {
		t.Fatalf("expected near-duplicate diff marker, got %q", got)
	}
	if !strings.Contains(got, "line19-changed") {
		t.Fatalf("expected the changed line in the diff, got %q", got)
	}
	if strings.Contains(got, "line0\n") {
		t.Fatalf("unchanged lines should not be repeated in the diff, got %q", got)
	}
}

func TestCheckCache_DissimilarOutputNoNearDup(t *testing.T) {
	t.Setenv("TOK_DATA_DIR", t.TempDir())
	cacheMu.Lock()
	cmdMap = map[string]cacheEntry{}
	cacheMu.Unlock()

	checkCache("docker", []string{"ps"}, "alpha\nbeta\ngamma\n")
	got := checkCache("docker", []string{"ps"}, "one\ntwo\nthree\n")

	if got != "" {
		t.Fatalf("completely different output should not be treated as near-duplicate, got %q", got)
	}
}
