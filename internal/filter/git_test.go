package filter

import (
	"strings"
	"testing"
)

var sampleGitStatus = `On branch main
Changes to be committed:
  (use "git restore --staged <file>..." to unstage)
	modified:   cmd/bench.go
	new file:   cmd/check.go

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
	modified:   README.md
	deleted:    old.go

Untracked files:
  (use "git add <file>..." to include in what will be committed)
	scratchpad.txt

`

func TestGitStatus_Branch(t *testing.T) {
	out := gitStatus(sampleGitStatus)
	if !strings.Contains(out, "main") {
		t.Errorf("expected branch name in output, got: %s", out)
	}
}

func TestGitStatus_StagedCount(t *testing.T) {
	out := gitStatus(sampleGitStatus)
	if !strings.Contains(out, "staged(2)") {
		t.Errorf("expected staged(2), got: %s", out)
	}
}

func TestGitStatus_ModifiedCount(t *testing.T) {
	out := gitStatus(sampleGitStatus)
	if !strings.Contains(out, "modified(2)") {
		t.Errorf("expected modified(2), got: %s", out)
	}
}

func TestGitStatus_Untracked(t *testing.T) {
	out := gitStatus(sampleGitStatus)
	if !strings.Contains(out, "untracked") {
		t.Errorf("expected untracked in output, got: %s", out)
	}
}

func TestGitStatus_Clean(t *testing.T) {
	in := "On branch main\nnothing to commit, working tree clean\n"
	out := gitStatus(in)
	if !strings.Contains(out, "clean") {
		t.Errorf("clean status should mention clean, got: %s", out)
	}
}

var sampleGitDiff = `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,8 @@
 package main
 
+import "fmt"
+
 func main() {
-	doOld()
+	fmt.Println("hello")
+	doNew()
 }
`

func TestGitDiff_PresentsChanges(t *testing.T) {
	out := gitDiff(sampleGitDiff)
	if out == "" {
		t.Fatal("gitDiff should return non-empty output")
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("expected filename in diff output, got: %s", out)
	}
}
