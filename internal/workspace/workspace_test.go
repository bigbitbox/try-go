package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFormatNewNameAndUniqueResolution(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

	if got := FormatNewName("my cool idea", false, now); got != "my-cool-idea" {
		t.Fatalf("expected undated my-cool-idea, got %q", got)
	}
	if got := FormatNewName("my cool idea", true, now); got != "2026-11-14-my-cool-idea" {
		t.Fatalf("expected dated name, got %q", got)
	}

	if err := os.Mkdir(filepath.Join(tmp, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveUniqueName(tmp, "demo"); got != "demo-2" {
		t.Fatalf("expected demo-2, got %q", got)
	}

	if err := os.Mkdir(filepath.Join(tmp, "v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveUniqueName(tmp, "v2"); got != "v3" {
		t.Fatalf("expected numeric bump v3, got %q", got)
	}
}

func TestParseGitURIAndCloneNaming(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

	info, err := ParseGitURI("https://github.com/chatwoot/chatwoot.git")
	if err != nil {
		t.Fatal(err)
	}
	// Undated clone avoids redundant "chatwoot-chatwoot" by default!
	if got := GenerateCloneDirName(info, "", tmp, false, now); got != "chatwoot" {
		t.Fatalf("expected clean repo name chatwoot, got %q", got)
	}

	// Collision falls back to <user>-<repo> when user != repo
	info2, err := ParseGitURI("git@github.com:tobi/try.git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "try"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := GenerateCloneDirName(info2, "", tmp, false, now); got != "tobi-try" {
		t.Fatalf("expected fallback tobi-try on collision, got %q", got)
	}

	pr, err := ParseGitURI("https://github.com/tobi/try/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	if pr.PRID != "42" || pr.Clone != "https://github.com/tobi/try.git" {
		t.Fatalf("unexpected PR parse: %+v", pr)
	}
	if got := GenerateCloneDirName(pr, "", tmp, false, now); got != "try-pr-42" {
		t.Fatalf("expected try-pr-42, got %q", got)
	}
}

func TestDeleteValidatesAllNamesBeforeMutation(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	os.Mkdir(filepath.Join(base, "safe"), 0755)
	os.Symlink(outside, filepath.Join(base, "link"))
	if err := Delete(base, []string{"safe", "../escape"}); err == nil {
		t.Fatal("accepted escape")
	}
	if _, err := os.Stat(filepath.Join(base, "safe")); err != nil {
		t.Fatal("partial deletion on validation failure")
	}
	if err := Delete(base, []string{"safe", "link"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("symlink target deleted")
	}
}
