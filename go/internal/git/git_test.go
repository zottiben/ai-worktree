package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "-c", "init.defaultBranch=main", "init", "-q")
	run(t, repo, "config", "user.email", "t@test.local")
	run(t, repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "-qm", "init")
	return repo
}

func TestGetDefaultBranchLocal(t *testing.T) {
	repo := newRepo(t)
	branch, err := GetDefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Errorf("GetDefaultBranch = %q, want main", branch)
	}
}

func TestIsDirtyDetectsUntracked(t *testing.T) {
	repo := newRepo(t)
	if dirty, err := IsDirty(repo); err != nil || dirty {
		t.Fatalf("fresh repo should be clean (dirty=%v err=%v)", dirty, err)
	}
	// An untracked file must count as dirty even if status hides untracked.
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirty, err := IsDirty(repo); err != nil || !dirty {
		t.Fatalf("untracked file should be dirty (dirty=%v err=%v)", dirty, err)
	}
}

func TestAddResetAndMergeCheck(t *testing.T) {
	repo := newRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")

	if err := AddWorktree(repo, wt, "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatalf("worktree not created: %v", err)
	}

	// A detached worktree at the default tip is merged into refs/heads/main.
	merged, err := IsHeadMergedIntoRef(wt, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if !merged {
		t.Error("worktree HEAD at default tip should be merged into main")
	}

	// Commit an unlanded change in the worktree; now it is NOT merged.
	if err := os.WriteFile(filepath.Join(wt, "feature.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, wt, "add", "-A")
	run(t, wt, "commit", "-qm", "feature")
	merged, err = IsHeadMergedIntoRef(wt, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if merged {
		t.Error("worktree with an unlanded commit should NOT be merged into main")
	}

	// ResetWorktree returns it to the clean, merged default tip.
	if err := ResetWorktree(wt, "main"); err != nil {
		t.Fatalf("ResetWorktree: %v", err)
	}
	if dirty, _ := IsDirty(wt); dirty {
		t.Error("reset worktree should be clean")
	}
}

func TestShortHashStable(t *testing.T) {
	a := ShortHash("git@github.com:zottiben/ai-worktree.git")
	b := ShortHash("git@github.com:zottiben/ai-worktree.git")
	if a != b {
		t.Error("ShortHash must be deterministic")
	}
	if len(a) != 6 {
		t.Errorf("ShortHash length = %d, want 6 hex chars", len(a))
	}
}
