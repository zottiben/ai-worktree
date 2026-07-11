package pool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneListsDisposableAsCandidate(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(poolDir, wt); err != nil {
		t.Fatal(err)
	}

	res, err := PruneWithOptions(repo, poolDir, PruneOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 1 {
		t.Errorf("want 1 prune candidate, got %d (skipped %d)", len(res.Candidates), len(res.Skipped))
	}
	// Dry run must not remove anything.
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("dry-run prune must not delete the worktree: %v", err)
	}
}

func TestPruneSkipsLeased(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	if _, err := AcquireLease(repo, poolDir, 4, nil, "h"); err != nil {
		t.Fatal(err)
	}
	res, err := PruneWithOptions(repo, poolDir, PruneOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("a leased worktree must never be a prune candidate, got %d", len(res.Candidates))
	}
}

func TestPruneSkipsDirty(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(poolDir, wt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := PruneWithOptions(repo, poolDir, PruneOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("a dirty worktree must not be a prune candidate, got %d", len(res.Candidates))
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Category != PruneSkipUncommitted {
		t.Errorf("want 1 skip categorized %q, got %+v", PruneSkipUncommitted, res.Skipped)
	}
}
