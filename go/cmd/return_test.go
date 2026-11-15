package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zottiben/ai-worktree/go/internal/pool"
)

func TestReturnCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "awt")
	build := exec.Command("go", "build", "-o", binary, "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build awt: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		scenario string
		want     string
	}{
		{"name-from-main", ""},
		{"name-from-sibling", ""},
		{"name-from-subdirectory", ""},
		{"relative-path", ""},
		{"symlink-alias", ""},
		{"current-subdirectory", ""},
		{"awt-dir-alias", ""},
		{"unknown-name", `no worktree named "7"`},
		{"existing-directory", "is an existing path here"},
		{"main-checkout", "not inside a worktree managed by awt"},
		{"outside-repository", "outside a git repository"},
		{"slot-directory", "not managed by awt"},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			repo, poolDir, target, sibling := newDestroyPool(t, "")
			t.Setenv("AWT_DIR", "")
			state, err := pool.ReadState(poolDir)
			if err != nil {
				t.Fatal(err)
			}
			for i := range state.Worktrees {
				state.Worktrees[i].Leased = true
				state.Worktrees[i].LeaseHolder = "agent"
			}
			if err := pool.WriteState(poolDir, state); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(target, alias); err != nil {
				t.Fatal(err)
			}
			subdirectory := filepath.Join(target, "src")
			if err := os.Mkdir(subdirectory, 0o755); err != nil {
				t.Fatal(err)
			}

			dir, args := repo, []string{"return", "5"}
			switch tc.scenario {
			case "name-from-sibling":
				dir = sibling
			case "name-from-subdirectory":
				dir = filepath.Join(repo, "docs")
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			case "relative-path":
				rel, err := filepath.Rel(repo, target)
				if err != nil {
					t.Fatal(err)
				}
				args = []string{"return", rel}
			case "symlink-alias":
				dir, args = t.TempDir(), []string{"return", alias}
			case "current-subdirectory":
				dir, args = subdirectory, []string{"return"}
			case "awt-dir-alias":
				t.Setenv("AWT_DIR", alias)
				dir, args = sibling, []string{"return"}
			case "unknown-name":
				args = []string{"return", "7"}
			case "existing-directory":
				if err := os.Mkdir(filepath.Join(repo, "5"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "main-checkout":
				dir, args = repo, []string{"return"}
			case "outside-repository":
				dir = t.TempDir()
			case "slot-directory":
				dir, args = poolDir, []string{"return", "5"}
			}

			command := exec.Command(binary, args...)
			command.Dir = dir
			out, err := command.CombinedOutput()
			state, readErr := pool.ReadState(poolDir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			leased := map[string]bool{}
			for _, wt := range state.Worktrees {
				leased[wt.Path] = wt.Leased
			}
			if !leased[sibling] {
				t.Fatalf("sibling lease must survive: %+v\n%s", state, out)
			}
			if tc.want == "" {
				if err != nil {
					t.Fatalf("awt %v: %v\n%s", args, err, out)
				}
				if leased[target] {
					t.Fatalf("target lease must be released: %+v\n%s", state, out)
				}
				return
			}
			if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("want error containing %q, got %v\n%s", tc.want, err, out)
			}
			if !leased[target] {
				t.Fatalf("rejected return must keep the target lease: %+v", state)
			}
			if tc.scenario == "existing-directory" && !strings.Contains(string(out), target) {
				t.Fatalf("collision error must name the worktree path %s:\n%s", target, out)
			}
		})
	}
}
