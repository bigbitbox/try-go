package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ScriptWarning = "# if you can read this, you didn't launch try from a shell function. run: try --help"

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func FormatScript(cmds []string) string {
	if len(cmds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(ScriptWarning)
	b.WriteByte('\n')
	for i, c := range cmds {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(c)
		if i < len(cmds)-1 {
			b.WriteString(" && \\\n")
		} else {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// NotifyTerminal runs optional integrations without interpolating shell code.
func NotifyTerminal(path string) {
	label := "try: " + StripDatePrefix(filepath.Base(path))
	invoke := func(name string, args ...string) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, name, args...).Run()
	}
	pane := os.Getenv("HERDR_PANE_ID")
	if os.Getenv("HERDR_ENV") == "1" && pane != "" {
		invoke("herdr", "pane", "report-metadata", pane, "--source", "try", "--title", label)
		if ws := os.Getenv("HERDR_WORKSPACE_ID"); strings.HasSuffix(pane, ":p1") && ws != "" {
			invoke("herdr", "workspace", "rename", ws, label)
		}
	} else if os.Getenv("CMUX_SOCKET_PATH") != "" || os.Getenv("CMUX_BUNDLE_ID") != "" {
		invoke("cmux", "rename-tab", label)
	}
}

func quoteForShell(s, shell string) string {
	if shell == "fish" {
		return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
	}
	return ShellQuote(s)
}

func ScriptCD(path string) []string {
	return []string{"cd " + quoteForShell(path, os.Getenv("TRY_SHELL"))}
}

func InitSnippet(shell, binPath, explicitPath string) string {
	qBin := quoteForShell(binPath, shell)
	switch shell {
	case "fish":
		pathPart := ""
		if explicitPath != "" {
			pathPart = " --path " + quoteForShell(explicitPath, shell)
		}
		return fmt.Sprintf(`function try
  set -lx TRY_SHELL fish
  set -l out (%s exec%s $argv | string collect)
  set -l rc $pipestatus[1]
  if test $rc -eq 0
    eval $out
  else
    test -n "$out"; and echo $out
    return $rc
  end
end
`, qBin, pathPart)
	default:
		pathPart := ""
		if explicitPath != "" {
			pathPart = " --path " + quoteForShell(explicitPath, shell)
		}
		return fmt.Sprintf(`try() {
  local out rc
  out=$(TRY_SHELL=sh %s exec%s "$@")
  rc=$?
  if [ $rc -eq 0 ]; then
    eval "$out"
  else
    [ -n "$out" ] && printf '%%s\n' "$out"
    return $rc
  fi
}
`, qBin, pathPart)
	}
}
