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

func TestPruneMissingCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "awt")
	build := exec.Command("go", "build", "-o", binary, "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build awt: %v\n%s", err, out)
	}
	for _, scenario := range []string{"hint", "cleanup", "without-status", "linked", "legacy-recovery"} {
		t.Run(scenario, func(t *testing.T) {
			repo, poolDir, live, missing := newDestroyPool(t, "")
			state, err := pool.ReadState(poolDir)
			if err != nil {
				t.Fatal(err)
			}
			state.Worktrees = state.Worktrees[:1]
			if scenario == "hint" {
				state.Worktrees[0].Leased = true
			}
			if err := pool.WriteState(poolDir, state); err != nil {
				t.Fatal(err)
			}
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = repo
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			named := filepath.Join(poolDir, "review-long-branch-name", "repo")
			outside := filepath.Join(t.TempDir(), "outside")
			git("worktree", "add", "--detach", named, "main")
			git("worktree", "add", "--detach", outside, "main")
			destroyGit(t, missing, "commit", "--allow-empty", "-qm", "unpublished detached work")
			head := git("-C", missing, "rev-parse", "HEAD")
			for _, path := range []string{missing, named, outside} {
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
			}
			before := git("worktree", "list", "--porcelain", "-z")
			run := func(args ...string) []byte {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = repo
				if scenario == "linked" {
					cmd.Dir = live
				}
				out, err := cmd.Output()
				if err != nil {
					if exit, ok := err.(*exec.ExitError); ok {
						t.Fatalf("awt %v: %v\n%s", args, err, exit.Stderr)
					}
					t.Fatal(err)
				}
				return out
			}
			if scenario != "without-status" {
				out := string(run("status"))
				if strings.Contains(out, "held by recovered:") || !strings.Contains(out, "awt prune --missing") {
					t.Fatalf("status must show concise recovery labels and a cleanup hint:\n%s", out)
				}
				column := -1
				for _, line := range strings.Split(out, "\n") {
					if at := strings.Index(line, "  missing"); at >= 0 {
						if column != -1 && at != column {
							t.Fatalf("status columns are not aligned:\n%s", out)
						}
						column = at
					}
				}
			}
			if scenario == "legacy-recovery" {
				legacy, err := pool.ReadState(poolDir)
				if err != nil {
					t.Fatal(err)
				}
				for i := range legacy.Worktrees {
					legacy.Worktrees[i].LeaseSource = ""
				}
				if err := pool.WriteState(poolDir, legacy); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "hint" {
				out := string(run("prune"))
				if !strings.Contains(out, "awt prune --missing") {
					t.Fatalf("prune must explain how to clean missing entries:\n%s", out)
				}
				return
			}
			for _, flag := range []string{"--all", "--global", "--prune-orphans"} {
				cmd := exec.Command(binary, "prune", "--missing", flag, "--yes")
				cmd.Dir = repo
				if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "cannot be combined") {
					t.Fatalf("unsafe flag combination was accepted: %s %v\n%s", flag, err, out)
				}
			}
			var preview jsonPruneResult
			if err := json.Unmarshal(run("prune", "--missing", "--json"), &preview); err != nil {
				t.Fatal(err)
			}
			if !preview.DryRun || len(preview.Candidates) != 2 || len(preview.Pruned) != 0 {
				t.Fatalf("want two missing candidates only: %+v", preview)
			}
			if got := git("worktree", "list", "--porcelain", "-z"); got != before {
				t.Fatal("dry run changed Git registrations")
			}
			if got := git("for-each-ref", "--format=%(refname)", "refs/awt/recovered/"); got != "" {
				t.Fatalf("dry run created recovery refs: %s", got)
			}
			if out := string(run("prune", "--missing")); !strings.Contains(out, "--missing --yes") {
				t.Fatalf("missing cleanup preview must show the execution command:\n%s", out)
			}
			var result jsonPruneResult
			if err := json.Unmarshal(run("prune", "--missing", "--yes", "--json"), &result); err != nil {
				t.Fatal(err)
			}
			if result.DryRun || len(result.Pruned) != 2 {
				t.Fatalf("want two missing entries removed: %+v", result)
			}
			if got := git("rev-parse", "refs/awt/recovered/"+head); got != head {
				t.Fatalf("unpublished HEAD was not preserved: %s", got)
			}
			after := git("worktree", "list", "--porcelain", "-z")
			if strings.Contains(after, missing) || strings.Contains(after, named) || !strings.Contains(after, outside) {
				t.Fatalf("cleanup must remove only missing registrations in this pool: %q", after)
			}
			if _, err := os.Stat(filepath.Join(live, ".git")); err != nil {
				t.Fatalf("live checkout changed: %v", err)
			}
			var status jsonStatusResult
			if err := json.Unmarshal(run("status", "--json"), &status); err != nil {
				t.Fatal(err)
			}
			if len(status.Worktrees) != 1 || status.Worktrees[0].Path != live {
				t.Fatalf("cleaned entries reappeared: %+v", status)
			}
			run("prune", "--missing", "--yes")
		})
	}
}
