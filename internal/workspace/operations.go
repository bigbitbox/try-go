package workspace

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ValidateName confines mutations to one visible entry under the workspace root.
func ValidateName(name string) error {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid directory name: %q", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("control character in directory name")
		}
	}
	return nil
}

func runGit(args ...string) error {
	c := exec.Command("git", args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if e := c.Run(); e != nil {
		return fmt.Errorf("git failed: %w", e)
	}
	return nil
}
func worktreeFile(path string) bool {
	st, e := os.Stat(filepath.Join(path, ".git"))
	return e == nil && st.Mode().IsRegular()
}
func directEntry(base, name string) (string, error) {
	if e := ValidateName(name); e != nil {
		return "", e
	}
	root, e := filepath.EvalSymlinks(ExpandPath(base))
	if e != nil {
		return "", e
	}
	p := filepath.Join(root, name)
	st, e := os.Lstat(p)
	if e != nil {
		return "", e
	}
	if !st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
		return "", errors.New("entry is not a directory")
	}
	return p, nil
}
func vacant(path string) error {
	if _, e := os.Lstat(path); e == nil {
		return fmt.Errorf("destination exists: %s", path)
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}

func Clone(base, uri, name string, dated bool) (string, error) {
	if name != "" {
		if e := ValidateName(strings.TrimSpace(name)); e != nil {
			return "", e
		}
	}
	info, e := ParseGitURI(uri)
	if strings.HasPrefix(uri, "file://") {
		u, err := url.Parse(uri)
		if err != nil {
			return "", err
		}
		info = &GitRepoInfo{Repo: strings.TrimSuffix(filepath.Base(u.Path), ".git"), Clone: uri}
		e = nil
	}
	if e != nil {
		return "", e
	}
	if e = ValidateName(info.Repo); e != nil {
		return "", e
	}
	if info.User != "" {
		if e = ValidateName(info.User); e != nil {
			return "", e
		}
	}
	path := filepath.Join(base, GenerateCloneDirName(info, name, base, dated, time.Now()))
	if e = os.MkdirAll(base, 0755); e != nil {
		return "", e
	}
	if e = runGit("clone", "--", info.Clone, path); e != nil {
		return "", e
	}
	if info.PRID != "" {
		if e = runGit("-C", path, "fetch", "origin", "pull/"+info.PRID+"/head"); e != nil {
			return "", e
		}
		if e = runGit("-C", path, "checkout", "--detach", "FETCH_HEAD"); e != nil {
			return "", e
		}
	}
	return path, nil
}

var ErrNotRepository = errors.New("not a Git repository")

func RepositoryRoot(path string) (string, error) {
	c := exec.Command("git", "-C", ExpandPath(path), "rev-parse", "--show-toplevel")
	c.Env = append(os.Environ(), "LC_ALL=C")
	out, e := c.Output()
	if e != nil {
		var exit *exec.ExitError
		if errors.As(e, &exit) && strings.Contains(string(exit.Stderr), "not a git repository") {
			return "", ErrNotRepository
		}
		return "", fmt.Errorf("cannot discover Git repository at %s: %w", path, e)
	}
	return strings.TrimSpace(string(out)), nil
}

func Worktree(base, repo, name string, dated bool) (string, error) {
	if name != "" {
		if e := ValidateName(strings.TrimSpace(name)); e != nil {
			return "", e
		}
	}
	repo = ExpandPath(repo)
	if e := os.MkdirAll(base, 0755); e != nil {
		return "", e
	}
	repo, e := RepositoryRoot(repo)
	if e != nil {
		return "", e
	}
	path := WorktreeTargetPath(base, repo, name, dated, time.Now())
	if e = runGit("-C", repo, "worktree", "add", "--detach", "--", path, "HEAD"); e != nil {
		return "", e
	}
	return path, nil
}

func moveEntry(source, dest string) error {
	if e := vacant(dest); e != nil {
		return e
	}
	st, e := os.Lstat(source)
	if e != nil {
		return e
	}
	if st.Mode()&os.ModeSymlink == 0 && worktreeFile(source) {
		return runGit("-C", source, "worktree", "move", "--", source, dest)
	}
	if e = os.Rename(source, dest); e != nil {
		return e
	}
	// Moving a main checkout can invalidate the backlinks of its linked worktrees.
	if st.IsDir() {
		if g, e := os.Stat(filepath.Join(dest, ".git")); e == nil && g.IsDir() {
			if e = runGit("-C", dest, "worktree", "repair"); e != nil {
				return fmt.Errorf("moved to %s but Git repair failed: %w", dest, e)
			}
		}
	}
	return nil
}
func Rename(base, oldName, newName string) (string, error) {
	src, e := directEntry(base, oldName)
	if e != nil {
		return "", e
	}
	if e = ValidateName(strings.TrimSpace(newName)); e != nil {
		return "", e
	}
	newName = SanitizeName(newName)
	dest := filepath.Join(filepath.Dir(src), newName)
	if src == dest {
		return dest, nil
	}
	return dest, moveEntry(src, dest)
}
func Graduate(base, name, dest string) (string, error) {
	src, e := directEntry(base, name)
	if e != nil {
		return "", e
	}
	st, e := os.Lstat(src)
	if e != nil {
		return "", e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("entry is already a symlink; graduate its original directory instead")
	}
	if strings.TrimSpace(dest) == "" {
		return "", errors.New("destination cannot be empty")
	}
	dest = ExpandPath(dest)
	parent, e := filepath.EvalSymlinks(filepath.Dir(dest))
	if e != nil {
		return "", e
	}
	dest = filepath.Join(parent, filepath.Base(dest))
	if dest == src || strings.HasPrefix(dest, src+string(filepath.Separator)) {
		return "", errors.New("destination is inside source")
	}
	if e = moveEntry(src, dest); e != nil {
		return "", e
	}
	if e = os.Symlink(dest, src); e != nil {
		return "", fmt.Errorf("moved to %s but symlink failed: %w", dest, e)
	}
	return dest, nil
}
func Delete(base string, names []string) error {
	if len(names) == 0 {
		return errors.New("nothing marked for deletion")
	}
	paths := make([]string, 0, len(names))
	seen := map[string]bool{}
	// Validate the complete set before deleting any entry.
	for _, name := range names {
		p, e := directEntry(base, name)
		if e != nil {
			return e
		}
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink == 0 {
			if g, err := os.Stat(filepath.Join(p, ".git")); err == nil && g.IsDir() {
				out, err := exec.Command("git", "-C", p, "worktree", "list", "--porcelain", "-z").Output()
				if err != nil {
					return fmt.Errorf("cannot inspect linked worktrees: %w", err)
				}
				count := 0
				for _, field := range strings.Split(string(out), "\x00") {
					if strings.HasPrefix(field, "worktree ") {
						count++
					}
				}
				if count > 1 {
					return fmt.Errorf("%s has linked worktrees; remove them before deleting the main repository", p)
				}
			}
		}
		if !seen[p] {
			paths = append(paths, p)
			seen[p] = true
		}
	}
	for _, p := range paths {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			e = os.Remove(p)
		} else if worktreeFile(p) {
			e = runGit("-C", p, "worktree", "remove", "--force", "--", p)
		} else {
			e = os.RemoveAll(p)
		}
		if e != nil {
			return e
		}
	}
	return nil
}

func Create(base, raw string, dated bool) (string, error) {
	if err := ValidateName(strings.TrimSpace(raw)); err != nil {
		return "", err
	}
	name := FormatNewName(raw, dated, time.Now())
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", err
	}
	for {
		path := filepath.Join(base, ResolveUniqueName(base, name))
		err := os.Mkdir(path, 0755)
		if os.IsExist(err) {
			continue
		}
		return path, err
	}
}
