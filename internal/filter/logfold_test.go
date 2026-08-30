package filter

import (
	"testing"
)

func TestLogfold_NoChange(t *testing.T) {
	in := "line one\nline two\nline three\n"
	if got := logfold(in); got != in {
		t.Fatalf("want %q, got %q", in, got)
	}
}

func TestLogfold_TwoDuplicatesNotCollapsed(t *testing.T) {
	in := "warn: foo\nwarn: foo\nwarn: bar\n"
	if got := logfold(in); got != in {
		t.Fatalf("two identical lines should not collapse, got %q", got)
	}
}

func TestLogfold_ThreeCollapsed(t *testing.T) {
	in := "warn: foo\nwarn: foo\nwarn: foo\nwarn: bar\n"
	want := "[3x] warn: foo\nwarn: bar\n"
	if got := logfold(in); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestLogfold_FiveCollapsed(t *testing.T) {
	in := "err\nerr\nerr\nerr\nerr\n"
	want := "[5x] err\n"
	if got := logfold(in); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestLogfold_MixedRuns(t *testing.T) {
	in := "a\na\na\nb\nb\na\n"
	want := "[3x] a\nb\nb\na\n"
	if got := logfold(in); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestLogfold_ShortInputSkipped(t *testing.T) {
	in := "a\na\na\n"
	// Three lines → logfold skips inputs with ≤3 lines.
	if got := logfold(in); got != in {
		t.Fatalf("short input should pass through, got %q", got)
	}
}

func TestLogfold_EmptyString(t *testing.T) {
	if got := logfold(""); got != "" {
		t.Fatalf("empty input should return empty, got %q", got)
	}
}

func TestLogfold_TrailingNewlinePreserved(t *testing.T) {
	in := "x\nx\nx\nx\ny\n"
	got := logfold(in)
	if got[len(got)-1] != '\n' {
		t.Fatalf("trailing newline lost: %q", got)
	}
}
