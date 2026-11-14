package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, b)
	}
	return string(b)
}
func repository(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	gitTest(t, d, "init", "-q")
	gitTest(t, d, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "initial")
	return d
}
func TestDeleteRefusesMainRepositoryWithLinkedWorktrees(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "main")
	if e := os.Rename(repository(t), repo); e != nil {
		t.Fatal(e)
	}
	external, e := Worktree(t.TempDir(), repo, "linked", false)
	if e != nil {
		t.Fatal(e)
	}
	if e = Delete(base, []string{"main"}); e == nil {
		t.Fatal("deleted repository backing an external worktree")
	}
	gitTest(t, external, "status", "--porcelain")
}

func TestGitHubPRCloneFetchesDetachedRefWithoutNetwork(t *testing.T) {
	repo := repository(t)
	gitTest(t, repo, "update-ref", "refs/pull/42/head", "HEAD")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+repo+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/acme/demo.git")
	path, e := Clone(t.TempDir(), "https://github.com/acme/demo/pull/42", "", false)
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Base(path) != "demo-pr-42" {
		t.Fatal(path)
	}
	if got := strings.TrimSpace(gitTest(t, path, "rev-parse", "HEAD")); got != strings.TrimSpace(gitTest(t, repo, "rev-parse", "refs/pull/42/head")) {
		t.Fatal("wrong PR revision")
	}
	c := exec.Command("git", "-C", path, "symbolic-ref", "-q", "HEAD")
	if c.Run() == nil {
		t.Fatal("PR checkout is not detached")
	}
}

func TestCloneLocalRepoWithQuotedName(t *testing.T) {
	repo := repository(t)
	base := t.TempDir()
	path, e := Clone(base, "file://"+repo, "demo'$(touch marker)", false)
	if e != nil {
		t.Fatal(e)
	}
	gitTest(t, path, "status", "--porcelain")
	if filepath.Base(path) != "demo'$(touch-marker)" {
		t.Fatalf("bad path %s", path)
	}
	if _, e = Clone(base, "file://"+repo, "../escape", false); e == nil {
		t.Fatal("accepted escape")
	}
}

func TestCreateRejectsEscapeAndReservesUniqueDirectory(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"../escape", "/absolute", ".", "..", ".hidden", "bad\nname", ""} {
		if _, e := Create(base, name, false); e == nil {
			t.Errorf("accepted %q", name)
		}
	}
	a, e := Create(base, "中文 idea", false)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Create(base, "中文 idea", false)
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Base(a) != "中文-idea" || filepath.Base(b) != "中文-idea-2" {
		t.Fatalf("unexpected names %s %s", a, b)
	}
}

func TestWorktreeRenameGraduateDeleteKeepsGitMetadata(t *testing.T) {
	repo := repository(t)
	base := t.TempDir()
	path, e := Worktree(base, repo, "trial", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(path, ".git")); e != nil {
		t.Fatal(e)
	}
	path, e = Rename(base, "trial", "renamed")
	if e != nil {
		t.Fatal(e)
	}
	gitTest(t, path, "status", "--porcelain")
	dest := filepath.Join(t.TempDir(), "graduate")
	path, e = Graduate(base, "renamed", dest)
	if e != nil {
		t.Fatal(e)
	}
	gitTest(t, path, "status", "--porcelain")
	if e = Delete(base, []string{"renamed"}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(dest); e != nil {
		t.Fatal("deleting graduated link deleted target", e)
	}
	// Deleting a real worktree unregisters it rather than leaving stale metadata.
	path, e = Worktree(base, repo, "another", false)
	if e != nil {
		t.Fatal(e)
	}
	if e = Delete(base, []string{"another"}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("worktree still exists", e)
	}
}
