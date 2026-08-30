package filter

import (
	"testing"
)

func TestResolve_AbsolutePath(t *testing.T) {
	cmd, args := resolve("/usr/bin/git", []string{"status"})
	if cmd != "git" {
		t.Fatalf("want git, got %q", cmd)
	}
	_ = args
}

func TestResolve_UvRun(t *testing.T) {
	cmd, args := resolve("uv", []string{"run", "pytest", "-v"})
	if cmd != "pytest" {
		t.Fatalf("want pytest, got %q", cmd)
	}
	if len(args) != 1 || args[0] != "-v" {
		t.Fatalf("want [-v], got %v", args)
	}
}

func TestResolve_NpxNoPrefix(t *testing.T) {
	cmd, args := resolve("npx", []string{"jest", "--watch"})
	if cmd != "jest" {
		t.Fatalf("want jest, got %q", cmd)
	}
	if len(args) != 1 || args[0] != "--watch" {
		t.Fatalf("want [--watch], got %v", args)
	}
}

func TestResolve_RunnerWithFlags(t *testing.T) {
	// uv run --python 3.12 pytest → skip runner flags, resolve to pytest
	cmd, args := resolve("uv", []string{"run", "--python", "3.12", "pytest"})
	if cmd != "pytest" {
		t.Fatalf("want pytest, got %q", cmd)
	}
	_ = args
}

func TestResolve_NoMatch(t *testing.T) {
	cmd, args := resolve("curl", []string{"-v", "https://example.com"})
	if cmd != "curl" {
		t.Fatalf("want curl, got %q", cmd)
	}
	if len(args) != 2 {
		t.Fatalf("want 2 args, got %v", args)
	}
}

func TestResolve_PnpmExec(t *testing.T) {
	cmd, args := resolve("pnpm", []string{"exec", "eslint", "src/"})
	if cmd != "eslint" {
		t.Fatalf("want eslint, got %q", cmd)
	}
	if len(args) != 1 || args[0] != "src/" {
		t.Fatalf("want [src/], got %v", args)
	}
}

func TestFilterable_KnownCmd(t *testing.T) {
	eff, matched := Filterable("git", []string{"status"})
	if !matched {
		t.Fatalf("git status should be filterable")
	}
	if eff != "git" {
		t.Fatalf("want eff=git, got %q", eff)
	}
}

func TestFilterable_UnknownCmd(t *testing.T) {
	_, matched := Filterable("curl", []string{"-v", "https://example.com"})
	if matched {
		t.Fatalf("curl should not be filterable")
	}
}

func TestFilterable_ResolvedRunner(t *testing.T) {
	eff, matched := Filterable("uv", []string{"run", "pytest", "-v"})
	if !matched {
		t.Fatalf("uv run pytest should be filterable")
	}
	if eff != "pytest" {
		t.Fatalf("want eff=pytest, got %q", eff)
	}
}
