package workspace

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	datePrefixRe   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)$`)
	whitespaceRe   = regexp.MustCompile(`\s+`)
	trailingNumRe  = regexp.MustCompile(`^(.*[^\d])(\d+)$`)
	githubPRRe     = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([^/]+)/([^/]+)/pull/(\d+)/?$`)
	httpsGitHostRe = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/(.+?)/?$`)
	sshURLGitRe    = regexp.MustCompile(`^ssh://(?:[^@/]+@)?([^/:]+)(?::\d+)?/([^/]+)/(.+?)/?$`)
	scpGitRe       = regexp.MustCompile(`^([^@/:]+)@([^:]+):([^/]+)/(.+?)/?$`)
	scpSimpleGitRe = regexp.MustCompile(`^([^@/:]+)@([^:]+):(.+?)/?$`)
)

type Entry struct {
	Basename   string
	EntryPath  string // Path inside basePath (may be symlink itself)
	TargetPath string // Resolved target path for cd
	IsSymlink  bool
	ModTime    time.Time
	BaseScore  float64
}

type GitRepoInfo struct {
	Host  string
	User  string
	Repo  string
	PRID  string
	Clone string
}

type Config struct {
	TriesPath    string
	ClonePath    string
	ProjectsPath string
	UseDate      bool
}

func ExpandPath(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func ResolveConfig(explicitPath string, flagDate bool) Config {
	home, _ := os.UserHomeDir()
	defaultIncubator := filepath.Join(home, "codebase", "incubator")
	defaultClone := filepath.Join(home, "Downloads", "git-repos")
	defaultRay := filepath.Join(home, "codebase", "Ray")

	var triesPath string
	hasCustomPath := false
	if explicitPath != "" {
		triesPath = ExpandPath(explicitPath)
		hasCustomPath = filepath.Clean(triesPath) != filepath.Clean(defaultIncubator)
	} else if envPath := strings.TrimSpace(os.Getenv("TRY_PATH")); envPath != "" {
		triesPath = ExpandPath(envPath)
		hasCustomPath = filepath.Clean(triesPath) != filepath.Clean(defaultIncubator)
	} else if dirExists(filepath.Join(home, "codebase")) {
		triesPath = defaultIncubator
	} else {
		triesPath = filepath.Join(home, "src", "tries")
	}

	var clonePath string
	if explicitPath != "" {
		clonePath = triesPath
	} else if envClone := strings.TrimSpace(os.Getenv("TRY_CLONE_PATH")); envClone != "" {
		clonePath = ExpandPath(envClone)
	} else if !hasCustomPath && home != "" && dirExists(filepath.Join(home, "Downloads")) {
		clonePath = defaultClone
	} else {
		clonePath = triesPath
	}

	var projectsPath string
	if envProj := strings.TrimSpace(os.Getenv("TRY_PROJECTS")); envProj != "" {
		projectsPath = ExpandPath(envProj)
	} else if filepath.Clean(triesPath) == filepath.Clean(defaultIncubator) && dirExists(defaultRay) {
		projectsPath = defaultRay
	} else {
		projectsPath = filepath.Dir(triesPath)
	}

	useDate := flagDate
	if !useDate {
		envDate := strings.ToLower(strings.TrimSpace(os.Getenv("TRY_DATE")))
		useDate = envDate == "1" || envDate == "true" || envDate == "yes"
	}

	return Config{
		TriesPath:    triesPath,
		ClonePath:    clonePath,
		ProjectsPath: projectsPath,
		UseDate:      useDate,
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func pathOccupied(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func SanitizeName(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "/")
	s = whitespaceRe.ReplaceAllString(s, "-")
	return s
}

func SplitDatePrefix(name string) (datePart string, rest string, hasDate bool) {
	m := datePrefixRe.FindStringSubmatch(name)
	if m == nil {
		return "", name, false
	}
	return m[1], m[2], true
}

func StripDatePrefix(name string) string {
	if _, rest, ok := SplitDatePrefix(name); ok {
		return rest
	}
	return name
}

func FormatNewName(raw string, useDate bool, now time.Time) string {
	clean := SanitizeName(raw)
	if clean == "" {
		return ""
	}
	if useDate {
		if _, _, already := SplitDatePrefix(clean); !already {
			return now.Format("2006-01-02") + "-" + clean
		}
	}
	return clean
}

func ResolveUniqueName(baseDir, desired string) string {
	if desired == "" {
		return ""
	}
	if !pathOccupied(filepath.Join(baseDir, desired)) {
		return desired
	}

	if m := trailingNumRe.FindStringSubmatch(desired); m != nil {
		stem := m[1]
		n, err := strconv.Atoi(m[2])
		if err == nil {
			for candNum := n + 1; ; candNum++ {
				cand := fmt.Sprintf("%s%d", stem, candNum)
				if !pathOccupied(filepath.Join(baseDir, cand)) {
					return cand
				}
			}
		}
	}

	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", desired, i)
		if !pathOccupied(filepath.Join(baseDir, cand)) {
			return cand
		}
	}
}

func LoadEntries(basePath string, now time.Time) ([]Entry, error) {
	dirEntries, err := os.ReadDir(basePath)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(basePath, name)
		lstat, err := os.Lstat(entryPath)
		if err != nil {
			continue
		}

		isSymlink := lstat.Mode()&os.ModeSymlink != 0
		targetPath := entryPath
		stat := lstat
		if isSymlink {
			resolved, err := filepath.EvalSymlinks(entryPath)
			if err != nil {
				continue
			}
			tstat, err := os.Stat(resolved)
			if err != nil || !tstat.IsDir() {
				continue
			}
			targetPath = resolved
			stat = tstat
		} else if !lstat.IsDir() {
			continue
		}

		mtime := stat.ModTime()
		hours := math.Max(0, now.Sub(mtime).Hours())
		baseScore := 3.0 / math.Sqrt(hours+1.0)

		entries = append(entries, Entry{
			Basename:   name,
			EntryPath:  entryPath,
			TargetPath: targetPath,
			IsSymlink:  isSymlink,
			ModTime:    mtime,
			BaseScore:  baseScore,
		})
	}
	return entries, nil
}

func IsGitURI(arg string) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}
	if strings.HasPrefix(arg, "http://") ||
		strings.HasPrefix(arg, "https://") ||
		strings.HasPrefix(arg, "ssh://") ||
		strings.HasPrefix(arg, "git@") {
		return true
	}
	if strings.Contains(arg, "github.com/") || strings.Contains(arg, "gitlab.com/") || strings.HasSuffix(arg, ".git") {
		return true
	}
	return scpGitRe.MatchString(arg)
}

func ParseGitURI(raw string) (*GitRepoInfo, error) {
	uri := strings.TrimSpace(raw)
	if uri == "" {
		return nil, errors.New("empty git URI")
	}

	if m := githubPRRe.FindStringSubmatch(uri); m != nil {
		user := m[1]
		repo := strings.TrimSuffix(m[2], ".git")
		prID := m[3]
		return &GitRepoInfo{
			Host:  "github.com",
			User:  user,
			Repo:  repo,
			PRID:  prID,
			Clone: fmt.Sprintf("https://github.com/%s/%s.git", user, repo),
		}, nil
	}

	trimmed := strings.TrimSuffix(strings.TrimRight(uri, "/"), ".git")

	if m := httpsGitHostRe.FindStringSubmatch(trimmed); m != nil {
		host, user, tail := m[1], m[2], m[3]
		repo := filepath.Base(tail)
		return &GitRepoInfo{Host: host, User: user, Repo: repo, Clone: uri}, nil
	}
	if m := sshURLGitRe.FindStringSubmatch(trimmed); m != nil {
		host, user, tail := m[1], m[2], m[3]
		repo := filepath.Base(tail)
		return &GitRepoInfo{Host: host, User: user, Repo: repo, Clone: uri}, nil
	}
	if m := scpGitRe.FindStringSubmatch(trimmed); m != nil {
		host, user, tail := m[2], m[3], m[4]
		repo := filepath.Base(tail)
		return &GitRepoInfo{Host: host, User: user, Repo: repo, Clone: uri}, nil
	}
	if m := scpSimpleGitRe.FindStringSubmatch(trimmed); m != nil {
		user, host, tail := m[1], m[2], m[3]
		repo := filepath.Base(tail)
		return &GitRepoInfo{Host: host, User: user, Repo: repo, Clone: uri}, nil
	}

	return nil, fmt.Errorf("unable to parse git URI: %s", raw)
}

func GenerateCloneDirName(info *GitRepoInfo, customName, baseDir string, useDate bool, now time.Time) string {
	if clean := SanitizeName(customName); clean != "" {
		formatted := FormatNewName(clean, useDate, now)
		return ResolveUniqueName(baseDir, formatted)
	}

	if useDate {
		var stem string
		if info.PRID != "" {
			stem = fmt.Sprintf("%s-%s-pr-%s", info.User, info.Repo, info.PRID)
		} else if info.User != "" {
			stem = fmt.Sprintf("%s-%s", info.User, info.Repo)
		} else {
			stem = info.Repo
		}
		return ResolveUniqueName(baseDir, FormatNewName(stem, true, now))
	}

	if info.PRID != "" {
		return ResolveUniqueName(baseDir, fmt.Sprintf("%s-pr-%s", info.Repo, info.PRID))
	}

	// Clean undated default: prefer <repo>; on collision fall back to <user>-<repo>, then <repo>-2.
	if !pathOccupied(filepath.Join(baseDir, info.Repo)) {
		return info.Repo
	}
	if info.User != "" && !strings.EqualFold(info.User, info.Repo) {
		userRepo := fmt.Sprintf("%s-%s", info.User, info.Repo)
		if !pathOccupied(filepath.Join(baseDir, userRepo)) {
			return userRepo
		}
	}
	return ResolveUniqueName(baseDir, info.Repo)
}

func WorktreeTargetPath(triesPath, repoDir, customName string, useDate bool, now time.Time) string {
	base := SanitizeName(customName)
	if base == "" {
		if realRepo, err := filepath.EvalSymlinks(repoDir); err == nil {
			base = filepath.Base(realRepo)
		} else {
			base = filepath.Base(repoDir)
		}
	}
	desired := FormatNewName(base, useDate, now)
	unique := ResolveUniqueName(triesPath, desired)
	return filepath.Join(triesPath, unique)
}

func FormatRelativeTime(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "?"
	}
	sec := now.Sub(t).Seconds()
	if sec < 60 {
		return "just now"
	}
	min := sec / 60
	if min < 60 {
		return fmt.Sprintf("%dm ago", int(min))
	}
	hr := min / 60
	if hr < 24 {
		return fmt.Sprintf("%dh ago", int(hr))
	}
	days := hr / 24
	if days < 7 {
		return fmt.Sprintf("%dd ago", int(days))
	}
	return fmt.Sprintf("%dw ago", int(days/7))
}
