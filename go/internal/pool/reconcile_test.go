package pool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zottiben/ai-worktree/go/internal/git"
)

func TestReconcileRecoversOnlyRegisteredPoolWorktrees(t *testing.T) {
	for _, stateFile := range []string{"missing", "empty", "corrupt"} {
		t.Run(stateFile, func(t *testing.T) {
			repo := newTestRepo(t)
			poolDir := newPoolDir(t)
			paths := []string{
				filepath.Join(poolDir, "5", filepath.Base(repo)),
				filepath.Join(poolDir, "review-branch", filepath.Base(repo)),
				filepath.Join(t.TempDir(), "outside"),
				filepath.Join(poolDir+"-other", "6", filepath.Base(repo)),
				filepath.Join(poolDir, "nested", "7", filepath.Base(repo)),
			}
			for _, path := range paths {
				if err := git.AddWorktree(repo, path, "main"); err != nil {
					t.Fatal(err)
				}
			}
			if stateFile != "missing" {
				body := `{"worktrees": []}`
				if stateFile == "corrupt" {
					body = `{"worktrees":`
				}
				if err := os.WriteFile(stateFilePath(poolDir), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				list, err := List(repo, poolDir)
				if err != nil {
					t.Fatal(err)
				}
				if len(list) != 2 {
					t.Fatalf("want only two pool worktrees, got %+v", list)
				}
				for i, wt := range list {
					if wt.Path != paths[i] || wt.Status != StatusLeased || wt.LeaseHolder == "" {
						t.Fatalf("recovered worktree is not protected: %+v", wt)
					}
				}
			}
			if _, err := Acquire(repo, poolDir, 2, nil); err == nil || !strings.Contains(err.Error(), "max_trees = 2") {
				t.Fatalf("recovered live worktrees must count towards capacity: %v", err)
			}
		})
	}
}

func TestMissingLeaseSurvivesCleanupAndRestoration(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)
	wt, err := AcquireLease(repo, poolDir, 1, nil, "original-holder")
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, wt, "commit", "--allow-empty", "-qm", "unpublished")
	head := gitRun(t, wt, "rev-parse", "HEAD")
	gitRun(t, repo, "worktree", "lock", "--reason", "temporarily offline", wt)
	saved := filepath.Join(t.TempDir(), "saved")
	if err := os.Rename(wt, saved); err != nil {
		t.Fatal(err)
	}
	before := gitRun(t, repo, "worktree", "list", "--porcelain", "-z")
	list, err := List(repo, poolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != StatusMissing || list[0].LeaseHolder != "original-holder" {
		t.Fatalf("missing lease lost: %+v", list)
	}
	pruned, err := Prune(repo, poolDir, false, nil)
	if err != nil || len(pruned.Pruned) != 0 {
		t.Fatalf("missing lease must not be pruned: %+v, %v", pruned, err)
	}
	destroyed, err := DestroyPool(poolDir, DestroyOptions{IncludeLeased: true, IncludeUnlanded: true, IncludeInUse: true})
	if err != nil || len(destroyed.Destroyed) != 0 || len(destroyed.Skipped) != 1 {
		t.Fatalf("missing lease must not be bulk-destroyed: %+v, %v", destroyed, err)
	}
	if _, err := AcquireLease(repo, poolDir, 1, nil, "new-holder"); err != nil {
		t.Fatalf("missing slot must not block capacity: %v", err)
	}
	if after := gitRun(t, repo, "worktree", "list", "--porcelain", "-z"); !strings.Contains(after, before) {
		t.Fatalf("acquire changed existing registration or HEAD:\nbefore %q\nafter %q", before, after)
	}
	if err := os.Rename(saved, wt); err != nil {
		t.Fatal(err)
	}
	list, err = List(repo, poolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Status != StatusLeased || list[0].LeaseHolder != "original-holder" {
		t.Fatalf("restored lease lost: %+v", list)
	}
	if got := gitRun(t, wt, "rev-parse", "HEAD"); got != head {
		t.Fatalf("unpublished commit changed: %s != %s", got, head)
	}
}

func TestReconcileMatchesSymlinkedPoolPaths(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)
	wt, err := AcquireLease(repo, poolDir, 2, nil, "keep")
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "pool-alias")
	if err := os.Symlink(poolDir, alias); err != nil {
		t.Fatal(err)
	}
	list, err := List(repo, alias)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Path != wt || list[0].LeaseHolder != "keep" {
		t.Fatalf("symlink created duplicate entries or lost lease: %+v", list)
	}
	if err := os.Remove(stateFilePath(poolDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	list, err = List(repo, alias)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != StatusMissing {
		t.Fatalf("missing registration not recovered through symlink: %+v", list)
	}
}
