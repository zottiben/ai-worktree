package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zottiben/ai-worktree/go/internal/pool"
)

func TestWorktreeRegistryCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "awt")
	build := exec.Command("go", "build", "-o", binary, "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build awt: %v\n%s", err, out)
	}

	for _, scenario := range []string{"status-existing", "status-missing", "get-missing", "get-recorded-missing", "get-without-status", "get-linked", "get-missing-gitfile"} {
		t.Run(scenario, func(t *testing.T) {
			repo, poolDir, first, second := newDestroyPool(t, "")
			state, err := pool.ReadState(poolDir)
			if err != nil {
				t.Fatal(err)
			}
			state.Worktrees[0].Leased = true
			state.Worktrees[0].LeaseHolder = "keep"
			if scenario != "get-recorded-missing" && scenario != "get-missing-gitfile" {
				state.Worktrees = state.Worktrees[:1]
			}
			if err := pool.WriteState(poolDir, state); err != nil {
				t.Fatal(err)
			}
			destroyGit(t, second, "commit", "--allow-empty", "-qm", "unpublished detached commit")
			before := exec.Command("git", "worktree", "list", "--porcelain", "-z")
			before.Dir = repo
			registrations, err := before.Output()
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "status-existing" {
				removePath := second
				if scenario == "get-missing-gitfile" {
					removePath = filepath.Join(second, ".git")
				}
				if err := os.RemoveAll(removePath); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(repo, "awt.toml"), []byte("max_trees = 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			run := func(args ...string) []byte {
				t.Helper()
				command := exec.Command(binary, args...)
				command.Dir = repo
				if scenario == "get-linked" {
					command.Dir = first
				}
				out, err := command.Output()
				if err != nil {
					if exit, ok := err.(*exec.ExitError); ok {
						t.Fatalf("awt %v: %v\n%s", args, err, exit.Stderr)
					}
					t.Fatal(err)
				}
				return out
			}
			if scenario != "get-without-status" {
				var status jsonStatusResult
				if err := json.Unmarshal(run("status", "--json"), &status); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(scenario, "status-") {
					if len(status.Worktrees) != 2 {
						t.Fatalf("status must include both registered pool worktrees, got %+v", status.Worktrees)
					}
					want := "missing"
					if scenario == "status-existing" {
						want = "leased"
					}
					if status.Worktrees[1].Path != second || status.Worktrees[1].Status != want {
						t.Fatalf("recovered worktree must be %s, got %+v", want, status.Worktrees[1])
					}
					if !strings.Contains(string(run("status")), want) {
						t.Fatalf("text status must show %s", want)
					}
					if scenario == "status-missing" {
						enter := exec.Command(binary, "enter", "--print-path", "6")
						enter.Dir = repo
						if out, err := enter.CombinedOutput(); err == nil || !strings.Contains(string(out), "missing from disk") {
							t.Fatalf("enter must reject a missing worktree: %v\n%s", err, out)
						}
					}
					return
				}
			}

			path := strings.TrimSpace(string(run("get", "--lease")))
			if path == first || path == second {
				t.Fatalf("must not reuse a leased or registered missing path: %s", path)
			}
			if filepath.Dir(filepath.Dir(path)) != poolDir {
				t.Fatalf("get must acquire from the repository pool %s, got %s", poolDir, path)
			}
			if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
				t.Fatal(err)
			}
			after := exec.Command("git", "worktree", "list", "--porcelain", "-z")
			after.Dir = repo
			updated, err := after.Output()
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range strings.Split(string(registrations), "\x00\x00") {
				if record != "" && !strings.Contains(string(updated), record) {
					t.Fatalf("existing registration or HEAD changed: %q\n%s", record, updated)
				}
			}
		})
	}
}
