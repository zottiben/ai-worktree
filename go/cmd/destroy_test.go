package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/zottiben/ai-worktree/go/internal/config"
	"github.com/zottiben/ai-worktree/go/internal/pool"
	"github.com/zottiben/ai-worktree/go/internal/process"
)

func destroyGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// newDestroyPool uses real linked worktrees, but never the user's config or pool.
func newDestroyPool(t *testing.T, root string) (repo, poolDir, target, sibling string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("AWT_NO_UPDATE_CHECK", "1")

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo = filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	destroyGit(t, repo, "init", "-q", "--initial-branch=main")
	destroyGit(t, repo, "config", "user.email", "t@test.local")
	destroyGit(t, repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "awt.toml"), []byte("root = \""+root+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destroyGit(t, repo, "add", "awt.toml")
	destroyGit(t, repo, "commit", "-qm", "init")
	poolDir, err = config.ResolvePoolDir(repo, root)
	if err != nil {
		t.Fatal(err)
	}
	var state pool.State
	for _, name := range []string{"5", "6"} {
		path := filepath.Join(poolDir, name, "repo")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		destroyGit(t, repo, "worktree", "add", "--detach", path, "main")
		state.Worktrees = append(state.Worktrees, pool.WorktreeEntry{Name: name, Path: path})
	}
	if err := pool.WriteState(poolDir, state); err != nil {
		t.Fatal(err)
	}
	return repo, poolDir, state.Worktrees[0].Path, state.Worktrees[1].Path
}

func runDestroy(t *testing.T, args ...string) (jsonDestroyResult, error) {
	t.Helper()
	destroyCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if err := flag.Value.Set(flag.DefValue); err != nil {
			t.Fatal(err)
		}
		flag.Changed = false
	})
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	stdout := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = stdout }()
	rootCmd.SetArgs(append([]string{"destroy", "--json"}, args...))
	defer rootCmd.SetArgs(nil)
	err = rootCmd.Execute()
	data, readErr := os.ReadFile(out.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	var result jsonDestroyResult
	if len(data) > 0 {
		if decodeErr := json.Unmarshal(data, &result); decodeErr != nil {
			t.Fatalf("decode destroy output %q: %v", data, decodeErr)
		}
	}
	return result, err
}

func TestDestroyExplicitPath(t *testing.T) {
	for _, form := range []string{"absolute", "relative", "current", "existing-bare-path", "symlink-alias"} {
		t.Run(form, func(t *testing.T) {
			repo, _, target, _ := newDestroyPool(t, "")
			arg := target
			switch form {
			case "absolute":
				t.Chdir(t.TempDir()) // Explicit paths also work outside a repository.
			case "relative":
				t.Chdir(repo)
				var err error
				arg, err = filepath.Rel(repo, target)
				if err != nil {
					t.Fatal(err)
				}
			case "current":
				t.Chdir(target)
				arg = "."
			case "existing-bare-path":
				t.Chdir(filepath.Dir(target))
				arg = filepath.Base(target)
			case "symlink-alias":
				arg = filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(target, arg); err != nil {
					t.Fatal(err)
				}
			}
			result, err := runDestroy(t, arg)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Planned)+len(result.Skipped) != 1 ||
				len(result.Planned) == 1 && result.Planned[0].Path != target ||
				len(result.Skipped) == 1 && result.Skipped[0].Path != target {
				t.Fatalf("expected only %s, got %+v", target, result)
			}
		})
	}
}

func TestDestroyRejectsInvalidTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown-name", []string{"7"}, "no worktree named"},
		{"explicit-missing-path", []string{"./5"}, "not managed by awt"},
		{"nested-missing-path", []string{"missing/5"}, "not managed by awt"},
		{"main-checkout", []string{"."}, "not managed by awt"},
		{"multiple-targets", []string{"5", "6"}, "at most 1 arg"},
		{"no-target", nil, "specify a worktree name or path"},
		{"bulk-no-target", []string{"--all"}, "requires a pool path"},
		{"bulk-name", []string{"5", "--all"}, "cannot resolve an awt pool"},
		{"bulk-leased", []string{".", "--all", "--include-leased"}, "cannot be combined"},
		{"existing-directory", []string{"5"}, "not managed by awt"},
		{"outside-repository", []string{"5"}, "outside a git repository"},
		{"other-repository", []string{"5"}, "no worktree named"},
		{"duplicate-name", []string{"5"}, "more than one worktree named"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, poolDir, target, sibling := newDestroyPool(t, "")
			t.Chdir(repo)
			switch tc.name {
			case "existing-directory":
				if err := os.Mkdir(filepath.Join(repo, "5"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "outside-repository", "other-repository":
				other := t.TempDir()
				if tc.name == "other-repository" {
					destroyGit(t, other, "init", "-q", "--initial-branch=main")
				}
				t.Chdir(other)
			case "duplicate-name":
				state, err := pool.ReadState(poolDir)
				if err != nil {
					t.Fatal(err)
				}
				state.Worktrees[1].Name = "5"
				if err := pool.WriteState(poolDir, state); err != nil {
					t.Fatal(err)
				}
			}
			result, err := runDestroy(t, append(tc.args, "--yes")...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
			if len(result.Destroyed) != 0 {
				t.Fatalf("invalid target removed worktrees: %+v", result)
			}
			for _, path := range []string{repo, target, sibling} {
				if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
					t.Fatalf("must preserve %s: %v", path, err)
				}
			}
		})
	}
}

func TestDestroyByNameKeepsRiskGates(t *testing.T) {
	for _, risk := range []string{"dirty", "unmerged", "leased", "in-use"} {
		t.Run(risk, func(t *testing.T) {
			repo, poolDir, target, _ := newDestroyPool(t, "")
			t.Chdir(repo)
			needed := pool.IncludeUnlandedFlag
			switch risk {
			case "dirty":
				if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("keep"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "unmerged":
				destroyGit(t, target, "commit", "--allow-empty", "-qm", "unlanded work")
			case "leased", "in-use":
				state, err := pool.ReadState(poolDir)
				if err != nil {
					t.Fatal(err)
				}
				if risk == "leased" {
					state.Worktrees[0].Leased = true
					needed = pool.IncludeLeasedFlag
				} else {
					pid := int32(os.Getpid())
					startedAt, ok := process.StartedAt(pid)
					if !ok {
						t.Fatal("cannot read test process identity")
					}
					state.Worktrees[0].OwnerPID = pid
					state.Worktrees[0].OwnerStartedAt = startedAt
					needed = pool.IncludeInUseFlag
				}
				if err := pool.WriteState(poolDir, state); err != nil {
					t.Fatal(err)
				}
			}
			result, err := runDestroy(t, "5", "--yes")
			if err == nil || !strings.Contains(err.Error(), needed) {
				t.Fatalf("want missing %s error, got %v", needed, err)
			}
			if len(result.Destroyed) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Path != target ||
				!slices.Contains(result.Skipped[0].NeededFlags, needed) {
				t.Fatalf("expected %s skip, got %+v", risk, result)
			}
			if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
				t.Fatalf("protected target must survive: %v", err)
			}
		})
	}
}

func TestDestroyByNameRemovesOnlyTarget(t *testing.T) {
	for _, name := range []string{"5", "review-feature", "leased-and-dirty"} {
		t.Run(name, func(t *testing.T) {
			repo, poolDir, target, sibling := newDestroyPool(t, "")
			t.Chdir(repo)
			state, err := pool.ReadState(poolDir)
			if err != nil {
				t.Fatal(err)
			}
			state.Worktrees[0].Name = name
			args := []string{name, "--yes"}
			if name == "leased-and-dirty" {
				state.Worktrees[0].Leased = true
				if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("disposable test data"), 0o644); err != nil {
					t.Fatal(err)
				}
				args = append(args, pool.IncludeLeasedFlag, pool.IncludeUnlandedFlag)
			}
			if err := pool.WriteState(poolDir, state); err != nil {
				t.Fatal(err)
			}
			result, err := runDestroy(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Destroyed) != 1 || result.Destroyed[0].Path != target {
				t.Fatalf("expected only %s removed, got %+v", target, result)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("target still exists: %v", err)
			}
			for _, path := range []string{repo, sibling} {
				if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
					t.Fatalf("must preserve %s: %v", path, err)
				}
			}
			state, err = pool.ReadState(poolDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Worktrees) != 1 || state.Worktrees[0].Path != sibling {
				t.Fatalf("expected only sibling in state, got %+v", state)
			}
		})
	}
}

func TestDestroyByName(t *testing.T) {
	for _, root := range []string{"", "worktrees"} {
		for _, location := range []string{"main", "worktree", "subdirectory"} {
			t.Run(root+"/"+location, func(t *testing.T) {
				repo, _, target, sibling := newDestroyPool(t, root)
				cwd := repo
				if location != "main" {
					cwd = target
				}
				if location == "subdirectory" {
					cwd = filepath.Join(target, "src")
					if err := os.Mkdir(cwd, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				t.Chdir(cwd)

				result, err := runDestroy(t, "5")
				if err != nil {
					t.Fatalf("destroy 5: %v", err)
				}
				if !result.DryRun || len(result.Destroyed) != 0 {
					t.Fatalf("expected dry run, got %+v", result)
				}
				if len(result.Planned)+len(result.Skipped) != 1 {
					t.Fatalf("expected exactly one target, got %+v", result)
				}
				if len(result.Planned) == 1 && result.Planned[0].Path != target ||
					len(result.Skipped) == 1 && result.Skipped[0].Path != target {
					t.Fatalf("resolved wrong worktree: %+v", result)
				}
				for _, path := range []string{target, sibling} {
					if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
						t.Fatalf("dry run must preserve %s: %v", path, err)
					}
				}
			})
		}
	}
}
