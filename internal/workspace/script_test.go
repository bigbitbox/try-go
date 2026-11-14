package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellOutputQuotesInjectionAndPreservesCdFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a'$(touch marker)")
	if e := os.Mkdir(dir, 0755); e != nil {
		t.Fatal(e)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			if _, e := exec.LookPath(shell); e != nil {
				t.Skip("shell not installed")
			}
			output, e := exec.Command(shell, "-c", FormatScript(ScriptCD(dir))+"pwd").CombinedOutput()
			if e != nil {
				t.Fatalf("%v: %s", e, output)
			}
			if !strings.Contains(string(output), dir) {
				t.Fatalf("unexpected pwd: %s", output)
			}
			t.Setenv("HERDR_ENV", "1")
			t.Setenv("HERDR_PANE_ID", "nonexistent:p1")
			if e = exec.Command(shell, "-c", FormatScript(ScriptCD(filepath.Join(dir, "missing")))).Run(); e == nil {
				t.Fatal("terminal hook masked failed cd")
			}
		})
	}
}

func TestInitSnippetPropagatesCommandFailure(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "try-go")
	if e := os.WriteFile(bin, []byte("#!/bin/sh\nexit 7\n"), 0755); e != nil {
		t.Fatal(e)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			if _, e := exec.LookPath(shell); e != nil {
				t.Skip("shell not installed")
			}
			e := exec.Command(shell, "-c", InitSnippet(shell, bin, "")+"try new demo").Run()
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 7 {
				t.Fatalf("lost failure status: %v", e)
			}
		})
	}
}

func TestNotifyTerminalUsesArgumentBoundaries(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$TRY_TEST_LOG\"\n"
	if e := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TRY_TEST_LOG", log)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PANE_ID", "demo:p1")
	t.Setenv("HERDR_WORKSPACE_ID", "ws")
	NotifyTerminal("/a/project'$(touch marker)")
	b, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b), "try: project'$(touch marker)\n") {
		t.Fatalf("bad arguments: %s", b)
	}
}
