package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveMissingWorktreeRechecksRegistration(t *testing.T) {
	for _, change := range []string{"lock", "head", "restored", "symlink"} {
		t.Run(change, func(t *testing.T) {
			repo := newRepo(t)
			path := filepath.Join(t.TempDir(), "wt")
			if err := AddWorktree(repo, path, "main"); err != nil {
				t.Fatal(err)
			}
			gitDir := run(t, path, "rev-parse", "--absolute-git-dir")
			saved := filepath.Join(t.TempDir(), "saved")
			if err := os.Rename(path, saved); err != nil {
				t.Fatal(err)
			}
			worktrees, err := ListLinkedWorktrees(repo)
			if err != nil || len(worktrees) != 1 || !worktrees[0].Prunable {
				t.Fatalf("want one prunable registration: %+v %v", worktrees, err)
			}
			switch change {
			case "lock":
				run(t, repo, "worktree", "lock", path)
			case "head":
				run(t, repo, "commit", "--allow-empty", "-qm", "new commit")
				head := run(t, repo, "rev-parse", "HEAD")
				run(t, repo, "--git-dir", gitDir, "update-ref", "HEAD", head)
			case "restored":
				if err := os.Rename(saved, path); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(saved, path); err != nil {
					t.Fatal(err)
				}
			}
			before := run(t, repo, "worktree", "list", "--porcelain", "-z")
			if err := RemoveMissingWorktree(repo, worktrees[0]); err == nil {
				t.Fatal("cleanup must reject changed or restored worktrees")
			}
			if after := run(t, repo, "worktree", "list", "--porcelain", "-z"); after != before {
				t.Fatal("refused cleanup changed the registration")
			}
		})
	}
}

func TestRemoveMissingWorktreeReusesRecoveryRef(t *testing.T) {
	repo := newRepo(t)
	for i := 0; i < 2; i++ {
		path := filepath.Join(t.TempDir(), "wt")
		if err := AddWorktree(repo, path, "main"); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		worktrees, err := ListLinkedWorktrees(repo)
		if err != nil || len(worktrees) != 1 {
			t.Fatalf("want one registration: %+v %v", worktrees, err)
		}
		if err := RemoveMissingWorktree(repo, worktrees[0]); err != nil {
			t.Fatal(err)
		}
		if got := run(t, repo, "rev-parse", worktrees[0].RecoveryRef()); got != worktrees[0].Head {
			t.Fatalf("HEAD not preserved: %s", got)
		}
	}
}
