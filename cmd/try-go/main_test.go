package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExecDoubleDashCreatesLeadingHyphenName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TRY_PATH", root)
	t.Setenv("TRY_DATE", "")
	if e := run([]string{"exec", "new", "--", "-wrapped"}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(root, "-wrapped")); e != nil {
		t.Fatal(e)
	}
}
func TestDotShortcutDiscoversRepositoryInSubdirectory(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "initial"}} {
		c := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, b)
		}
	}
	sub := filepath.Join(repo, "nested")
	if e := os.Mkdir(sub, 0755); e != nil {
		t.Fatal(e)
	}
	t.Chdir(sub)
	root := t.TempDir()
	t.Setenv("TRY_PATH", root)
	t.Setenv("TRY_DATE", "")
	if e := run([]string{"exec", ".", "from-nested"}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(root, "from-nested", ".git")); e != nil {
		t.Fatal("created empty directory instead of worktree:", e)
	}
}

func TestCLICompleteLifecycleAndExecOutput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TRY_PATH", root)
	t.Setenv("TRY_PROJECTS", t.TempDir())
	t.Setenv("TRY_CLONE_PATH", t.TempDir())
	t.Setenv("TRY_DATE", "")
	if e := run([]string{"exec", "new", "demo"}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(root, "demo")); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"exec", "rename", "demo", "changed"}); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"exec", "graduate", "changed"}); e != nil {
		t.Fatal(e)
	}
	if st, e := os.Lstat(filepath.Join(root, "changed")); e != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("missing graduate symlink", e)
	}
	if e := run([]string{"exec", "delete", "changed"}); e == nil {
		t.Fatal("delete without confirmation accepted")
	}
	if e := run([]string{"exec", "delete", "changed", "--yes"}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(os.Getenv("TRY_PROJECTS"), "changed")); e != nil {
		t.Fatal("deleted graduate target", e)
	}
}
