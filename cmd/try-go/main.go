// try-go manages workspaces; stdout is reserved for shell integration.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bigbitbox/try-go/internal/ui"
	"github.com/bigbitbox/try-go/internal/workspace"
)

const version = "0.1.0"
const help = `try-go — workspace manager (Go)

  try                         Search/select/create interactively
  try QUERY                   Start with a search
  try new NAME                Create a directory without a date
  try clone URL [NAME]         Clone into ~/Downloads/git-repos
  try URL [NAME]               Clone shortcut (GitHub PR URLs supported)
  try worktree [NAME]          Detached worktree from current repository
  try worktree NAME --repo DIR Use another repository
  try . NAME                  Worktree, or empty trial outside Git
  try rename OLD NEW          Rename a workspace
  try graduate NAME [DEST]    Move to ~/codebase/Ray; leave a symlink
  try delete NAME... --yes    Permanently remove entries; unlink symlinks only
  try init [zsh|bash|fish]     Print shell integration
  try install [SHELL]         Append integration to shell config once

Options: --path DIR (overrides both roots), --date (opt-in date prefix),
         --repo DIR, --help, --version; options may follow the command.
Environment: TRY_PATH, TRY_CLONE_PATH, TRY_PROJECTS, TRY_DATE
Keys: ↑/↓ or Ctrl-P/N · Enter · Ctrl-T new · Ctrl-R rename
      Ctrl-G graduate · Ctrl-D mark delete · YES confirms · Esc cancel

Build: go build -o bin/try-go ./cmd/try-go
Enable: eval "$(try-go init zsh)"
Git is required for clone/worktree operations; Ruby is not required.
`

type options struct {
	path, repo      string
	date, yes, exec bool
	args            []string
}

func parse(args []string) (options, error) {
	var o options
parseArgs:
	for i := 0; i < len(args); i++ {
		s := args[i]
		switch {
		case s == "--":
			o.args = append(o.args, args[i+1:]...)
			break parseArgs
		case s == "--date":
			o.date = true
		case s == "--yes":
			o.yes = true
		case s == "--path" || s == "--repo":
			if i+1 >= len(args) || args[i+1] == "" {
				return o, fmt.Errorf("%s requires a value", s)
			}
			i++
			if s == "--path" {
				o.path = args[i]
			} else {
				o.repo = args[i]
			}
		case strings.HasPrefix(s, "--path="):
			o.path = strings.TrimPrefix(s, "--path=")
			if o.path == "" {
				return o, errors.New("--path requires a value")
			}
		case strings.HasPrefix(s, "--repo="):
			o.repo = strings.TrimPrefix(s, "--repo=")
		case s == "--help" || s == "-h":
			o.args = []string{"help"}
			return o, nil
		case s == "--version" || s == "-v":
			o.args = []string{"version"}
			return o, nil
		case strings.HasPrefix(s, "-"):
			return o, fmt.Errorf("unknown option: %s (use -- before a name beginning with -)", s)
		default:
			o.args = append(o.args, s)
		}
	}
	if len(o.args) > 0 && o.args[0] == "exec" {
		o.exec = true
		o.args = o.args[1:]
	}
	return o, nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		if !errors.Is(e, errCancelled) {
			fmt.Fprintln(os.Stderr, "try:", e)
		}
		os.Exit(1)
	}
}

var errCancelled = errors.New("cancelled")

func run(args []string) error {
	o, e := parse(args)
	if e != nil {
		return e
	}
	cfg := workspace.ResolveConfig(o.path, o.date)
	if len(o.args) > 0 {
		switch o.args[0] {
		case "help":
			fmt.Fprint(os.Stderr, help)
			return nil
		case "version":
			fmt.Fprintln(os.Stderr, "try-go", version)
			return nil
		case "init", "install":
			return shellSetup(o.args, cfg)
		}
	}
	a := o.args
	if len(a) > 0 && a[0] == "cd" {
		a = a[1:]
	}
	var path string
	switch {
	case len(a) > 0 && a[0] == "new":
		if len(a) != 2 {
			return errors.New("usage: try new NAME")
		}
		path, e = workspace.Create(cfg.TriesPath, a[1], cfg.UseDate)
	case len(a) > 0 && (a[0] == "clone" || workspace.IsGitURI(a[0]) || strings.HasPrefix(a[0], "file://")):
		if a[0] == "clone" {
			a = a[1:]
		}
		if len(a) < 1 || len(a) > 2 {
			return errors.New("usage: try clone URL [NAME]")
		}
		name := ""
		if len(a) == 2 {
			name = a[1]
		}
		path, e = workspace.Clone(cfg.ClonePath, a[0], name, cfg.UseDate)
	case len(a) > 0 && a[0] == "worktree":
		rest := a[1:]
		repo := o.repo
		if repo == "" {
			repo = "."
		}
		name := ""
		if len(rest) > 0 {
			if st, err := os.Stat(workspace.ExpandPath(rest[0])); err == nil && st.IsDir() && o.repo == "" {
				repo = rest[0]
				rest = rest[1:]
			}
		}
		if len(rest) > 1 {
			return errors.New("usage: try worktree [NAME] --repo DIR")
		}
		if len(rest) == 1 {
			name = rest[0]
		}
		path, e = workspace.Worktree(cfg.TriesPath, repo, name, cfg.UseDate)
	case len(a) > 0 && (a[0] == "." || strings.HasPrefix(a[0], "./") || strings.HasPrefix(a[0], "../")):
		if len(a) > 2 || a[0] == "." && len(a) != 2 {
			return errors.New("usage: try . NAME")
		}
		repo := workspace.ExpandPath(a[0])
		name := filepath.Base(repo)
		if len(a) == 2 {
			name = a[1]
		}
		if discovered, err := workspace.RepositoryRoot(repo); err == nil {
			path, e = workspace.Worktree(cfg.TriesPath, discovered, name, cfg.UseDate)
		} else if errors.Is(err, workspace.ErrNotRepository) {
			path, e = workspace.Create(cfg.TriesPath, name, cfg.UseDate)
		} else {
			return err
		}
	case len(a) > 0 && a[0] == "rename":
		if len(a) != 3 {
			return errors.New("usage: try rename OLD NEW")
		}
		path, e = workspace.Rename(cfg.TriesPath, a[1], a[2])
	case len(a) > 0 && a[0] == "graduate":
		if len(a) < 2 || len(a) > 3 {
			return errors.New("usage: try graduate NAME [DEST]")
		}
		dest := filepath.Join(cfg.ProjectsPath, workspace.StripDatePrefix(a[1]))
		if len(a) == 3 {
			dest = a[2]
		}
		if e = os.MkdirAll(cfg.ProjectsPath, 0755); e == nil {
			path, e = workspace.Graduate(cfg.TriesPath, a[1], dest)
		}
	case len(a) > 0 && a[0] == "delete":
		if len(a) < 2 || !o.yes {
			return errors.New("usage: try delete NAME... --yes (permanent deletion)")
		}
		e = workspace.Delete(cfg.TriesPath, a[1:])
		path = cfg.TriesPath
	default:
		var action ui.Action
		action, e = ui.Run(cfg, strings.Join(a, " "))
		if e != nil {
			return e
		}
		switch action.Kind {
		case "":
			return errCancelled
		case "cd":
			path = action.Entry.TargetPath
		case "new":
			path, e = workspace.Create(cfg.TriesPath, action.Name, cfg.UseDate)
		case "rename":
			path, e = workspace.Rename(cfg.TriesPath, action.Entry.Basename, action.Name)
		case "graduate":
			if e = os.MkdirAll(cfg.ProjectsPath, 0755); e == nil {
				path, e = workspace.Graduate(cfg.TriesPath, action.Entry.Basename, action.Destination)
			}
		case "delete":
			e = workspace.Delete(cfg.TriesPath, action.Names)
			path = cfg.TriesPath
		default:
			return fmt.Errorf("unknown UI action %q", action.Kind)
		}
	}
	if e != nil {
		return e
	}
	if path == "" {
		return errors.New("missing target path")
	}
	// Match original recency semantics on successful navigation, without touching symlink targets on delete.
	now := time.Now()
	if e = os.Chtimes(path, now, now); e != nil {
		fmt.Fprintln(os.Stderr, "try: cannot update recency:", e)
	}
	workspace.NotifyTerminal(path)
	fmt.Print(workspace.FormatScript(workspace.ScriptCD(path)))
	return nil
}

func shellSetup(args []string, cfg workspace.Config) error {
	shell := filepath.Base(os.Getenv("SHELL"))
	explicit := ""
	if len(args) > 1 {
		if args[1] == "zsh" || args[1] == "bash" || args[1] == "fish" {
			shell = args[1]
		} else {
			explicit = workspace.ExpandPath(args[1])
		}
	}
	if shell != "zsh" && shell != "bash" && shell != "fish" {
		return errors.New("supported shells: zsh, bash, fish")
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	snippet := workspace.InitSnippet(shell, exe, explicit)
	if args[0] == "init" {
		fmt.Print(snippet)
		return nil
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	rc := filepath.Join(home, "."+shell+"rc")
	if shell == "fish" {
		rc = filepath.Join(home, ".config", "fish", "config.fish")
	}
	old, e := os.ReadFile(rc)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	marker := "# try-go shell integration"
	if strings.Contains(string(old), marker) {
		fmt.Fprintln(os.Stderr, "try-go integration already installed:", rc)
		return nil
	}
	if e = os.MkdirAll(filepath.Dir(rc), 0755); e != nil {
		return e
	}
	f, e := os.OpenFile(rc, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	_, e = f.WriteString("\n" + marker + "\n" + snippet)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintln(os.Stderr, "Added shell integration:", rc)
	return nil
}
