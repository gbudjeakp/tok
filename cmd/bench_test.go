package cmd

import (
	"testing"
)

func TestEstimateToks(t *testing.T) {
	cases := []struct {
		in      string
		wantMin int
		wantMax int
	}{
		{"", 0, 1},
		{"hello world", 2, 5},
		// ~3.5 chars/token; 350 chars → ~100 tokens
		{repeatStr("x", 350), 90, 110},
	}
	for _, c := range cases {
		got := estimateToks(c.in)
		if got < c.wantMin || got > c.wantMax {
			t.Errorf("estimateToks(%d chars): got %d, want %d..%d", len(c.in), got, c.wantMin, c.wantMax)
		}
	}
}

func TestPct(t *testing.T) {
	cases := []struct{ raw, filtered, want int }{
		{100, 50, 50},
		{100, 100, 0},
		{100, 0, 100},
		{0, 0, 0},      // zero-division guard
		{100, 120, -20}, // expanded
	}
	for _, c := range cases {
		if got := pct(c.raw, c.filtered); got != c.want {
			t.Errorf("pct(%d,%d): want %d, got %d", c.raw, c.filtered, c.want, got)
		}
	}
}

func TestFmtSaved(t *testing.T) {
	cases := []struct{ in int; want string }{
		{50, "-50%"},
		{0, "0%"},
		{-10, "+10%"},
	}
	for _, c := range cases {
		if got := fmtSaved(c.in); got != c.want {
			t.Errorf("fmtSaved(%d): want %q, got %q", c.in, c.want, got)
		}
	}
}

func repeatStr(s string, n int) string {
	out := make([]byte, n*len(s))
	for i := range out {
		out[i] = s[i%len(s)]
	}
	return string(out)
}
