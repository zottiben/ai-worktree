package pool

import (
	"os"
	"path/filepath"
	"testing"
)

const testMergeRef = "refs/heads/main"

func TestClassifyDisposable(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Return it: idle, clean, detached at the merged default tip.
	if err := Release(poolDir, wt); err != nil {
		t.Fatal(err)
	}

	state, _ := ReadState(poolDir)
	target := classifyForDestroy(state.Worktrees[0], testMergeRef)
	if !target.hasClass(DestroyDisposable) {
		t.Errorf("want disposable, got classes %v", target.Classes)
	}
}

func TestClassifyLeased(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	if _, err := AcquireLease(repo, poolDir, 4, nil, "h"); err != nil {
		t.Fatal(err)
	}
	state, _ := ReadState(poolDir)
	target := classifyForDestroy(state.Worktrees[0], testMergeRef)
	if !target.hasClass(DestroyLeased) {
		t.Errorf("want leased, got classes %v", target.Classes)
	}
}

func TestClassifyDirty(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(poolDir, wt); err != nil {
		t.Fatal(err)
	}
	// Introduce an untracked change.
	if err := os.WriteFile(filepath.Join(wt, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, _ := ReadState(poolDir)
	target := classifyForDestroy(state.Worktrees[0], testMergeRef)
	if !target.hasClass(DestroyDirty) {
		t.Errorf("want dirty, got classes %v", target.Classes)
	}
}

// TestMissingFlags checks the per-risk opt-in gating without touching disk.
func TestMissingFlags(t *testing.T) {
	leased := DestroyTarget{Class: DestroyLeased, Classes: []DestroyClass{DestroyLeased}}

	// Bulk (allowLeased=false): a lease can NEVER be removed by --all, even with
	// IncludeLeased set.
	bulk := DestroyOptions{IncludeLeased: true}
	if m := bulk.missingFlags(leased, false); len(m) != 1 || m[0] != IncludeLeasedFlag {
		t.Errorf("bulk leased should still require the flag, got %v", m)
	}
	// Named (allowLeased=true) with IncludeLeased: authorized.
	if m := bulk.missingFlags(leased, true); len(m) != 0 {
		t.Errorf("named leased with --include-leased should be allowed, got %v", m)
	}

	// Dirty is gated by --include-unlanded.
	dirty := DestroyTarget{Class: DestroyDirty, Classes: []DestroyClass{DestroyDirty}}
	if m := (DestroyOptions{}).missingFlags(dirty, false); len(m) != 1 || m[0] != IncludeUnlandedFlag {
		t.Errorf("dirty should require --include-unlanded, got %v", m)
	}

	// Disposable requires no flags.
	disp := DestroyTarget{Class: DestroyDisposable, Classes: []DestroyClass{DestroyDisposable}}
	if m := (DestroyOptions{}).missingFlags(disp, false); len(m) != 0 {
		t.Errorf("disposable should need no flags, got %v", m)
	}
}

// TestDestroyPoolNeverRemovesLeased proves a bulk --all destroy leaves a leased
// worktree in place and reports it as a leased-bulk skip.
func TestDestroyPoolNeverRemovesLeased(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := AcquireLease(repo, poolDir, 4, nil, "h")
	if err != nil {
		t.Fatal(err)
	}

	result, err := DestroyPool(poolDir, DestroyOptions{DryRun: false, IncludeLeased: true, IncludeUnlanded: true, IncludeInUse: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Destroyed) != 0 {
		t.Errorf("bulk destroy removed %d worktrees, want 0 (leased is never bulk-removable)", len(result.Destroyed))
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("leased worktree must survive bulk destroy: %v", err)
	}
	foundLeasedBulk := false
	for _, s := range result.Skipped {
		if s.LeasedBulk {
			foundLeasedBulk = true
		}
	}
	if !foundLeasedBulk {
		t.Error("expected a LeasedBulk skip entry")
	}
}
