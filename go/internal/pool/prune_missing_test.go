package pool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zottiben/ai-worktree/go/internal/git"
)

func TestPruneMissingProtectsUnsafeEntries(t *testing.T) {
	for _, scenario := range []string{"explicit-lease", "explicit-recovery-label", "git-lock", "owner", "existing-directory", "dangling-symlink", "outside-pool"} {
		t.Run(scenario, func(t *testing.T) {
			repo := newTestRepo(t)
			poolDir := newPoolDir(t)
			holder := "explicit-holder"
			if scenario == "explicit-recovery-label" {
				holder = unrecordedLeaseHolder
			}
			wt, err := AcquireLease(repo, poolDir, 4, nil, holder)
			if err != nil {
				t.Fatal(err)
			}
			state := mustReadState(t, poolDir)
			if scenario != "explicit-lease" && scenario != "explicit-recovery-label" {
				state.Worktrees[0].LeaseHolder = unrecordedLeaseHolder
				state.Worktrees[0].LeaseSource = leaseSourceRecovery
			}
			switch scenario {
			case "git-lock":
				gitRun(t, repo, "worktree", "lock", wt)
			case "owner":
				if err := reserveOwner(&state.Worktrees[0]); err != nil {
					t.Fatal(err)
				}
			case "outside-pool":
				outside := filepath.Join(t.TempDir(), "outside")
				gitRun(t, repo, "worktree", "move", wt, outside)
				wt = outside
				state.Worktrees[0].Path = outside
			}
			if err := os.RemoveAll(wt); err != nil {
				t.Fatal(err)
			}
			if scenario == "existing-directory" {
				if err := os.MkdirAll(wt, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(wt, "keep"), []byte("data"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "dangling-symlink" {
				if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), wt); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteState(poolDir, state); err != nil {
				t.Fatal(err)
			}
			before := gitRun(t, repo, "worktree", "list", "--porcelain", "-z")
			result, err := PruneMissing(repo, poolDir, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Pruned) != 0 || len(result.Candidates) != 0 {
				t.Fatalf("unsafe entry was included: %+v", result)
			}
			if got := gitRun(t, repo, "worktree", "list", "--porcelain", "-z"); got != before {
				t.Fatal("protected registration changed")
			}
			if got := mustReadState(t, poolDir); len(got.Worktrees) != 1 || got.Worktrees[0].LeaseHolder != state.Worktrees[0].LeaseHolder {
				t.Fatalf("protected state changed: %+v", got)
			}
			if scenario == "existing-directory" {
				if body, err := os.ReadFile(filepath.Join(wt, "keep")); err != nil || string(body) != "data" {
					t.Fatalf("remaining checkout files changed: %q %v", body, err)
				}
			}
			if scenario == "dangling-symlink" {
				if info, err := os.Lstat(wt); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("dangling symlink changed: %v", err)
				}
			}
		})
	}
}

func TestPruneMissingRecoversLegacyHoldsAndStateOnlyEntries(t *testing.T) {
	for _, holder := range []string{unrecordedLeaseHolder, recoveredLeaseHolder, ""} {
		t.Run(holder, func(t *testing.T) {
			repo := newTestRepo(t)
			poolDir := newPoolDir(t)
			wt, err := AcquireLease(repo, poolDir, 4, nil, holder)
			if err != nil {
				t.Fatal(err)
			}
			state := mustReadState(t, poolDir)
			state.Worktrees[0].Leased = holder != ""
			state.Worktrees[0].LeaseSource = ""
			if err := WriteState(poolDir, state); err != nil {
				t.Fatal(err)
			}
			if err := git.RemoveCleanWorktree(repo, wt); err != nil {
				t.Fatal(err)
			}
			result, err := PruneMissing(repo, poolDir, false)
			if err != nil || len(result.Pruned) != 1 {
				t.Fatalf("state-only entry not removed: %+v %v", result, err)
			}
			if got := mustReadState(t, poolDir); len(got.Worktrees) != 0 {
				t.Fatalf("stale state remains: %+v", got)
			}
		})
	}
}

func TestPruneMissingRefFailureKeepsRegistrationAndState(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)
	wt := filepath.Join(poolDir, "1", filepath.Base(repo))
	if err := git.AddWorktree(repo, wt, "main"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, wt, "commit", "--allow-empty", "-qm", "unpublished")
	head := gitRun(t, wt, "rev-parse", "HEAD")
	gitRun(t, repo, "update-ref", "refs/awt/recovered/"+head, "main")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	result, err := PruneMissing(repo, poolDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pruned) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Detail, "cannot preserve HEAD") {
		t.Fatalf("recovery failure was not reported: %+v", result)
	}
	if got := mustReadState(t, poolDir); len(got.Worktrees) != 1 {
		t.Fatalf("failed cleanup removed state: %+v", got)
	}
	if got := gitRun(t, repo, "worktree", "list", "--porcelain"); !strings.Contains(got, filepath.Base(filepath.Dir(wt))) {
		t.Fatalf("failed cleanup removed registration: %s", got)
	}
}

func TestPruneMissingHeadSurvivesGitGC(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)
	wt := filepath.Join(poolDir, "1", filepath.Base(repo))
	if err := git.AddWorktree(repo, wt, "main"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, wt, "commit", "--allow-empty", "-qm", "unpublished")
	head := gitRun(t, wt, "rev-parse", "HEAD")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	result, err := PruneMissing(repo, poolDir, false)
	if err != nil || len(result.Pruned) != 1 {
		t.Fatalf("cleanup failed: %+v %v", result, err)
	}
	gitRun(t, repo, "reflog", "expire", "--expire=now", "--all")
	gitRun(t, repo, "gc", "--prune=now")
	if got := gitRun(t, repo, "rev-parse", "refs/awt/recovered/"+head+"^{commit}"); got != head {
		t.Fatalf("unpublished commit was not preserved: %s", got)
	}
}
