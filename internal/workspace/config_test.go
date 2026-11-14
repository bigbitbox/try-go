package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitPathOverridesCloneEnvironment(t *testing.T) {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "codebase", "incubator")
	t.Setenv("TRY_CLONE_PATH", t.TempDir())
	cfg := ResolveConfig(base, false)
	if cfg.ClonePath != base {
		t.Fatalf("explicit --path ignored for clone: %+v", cfg)
	}
}
