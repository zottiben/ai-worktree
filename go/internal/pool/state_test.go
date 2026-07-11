package pool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	poolDir := t.TempDir()
	s := State{Worktrees: []WorktreeEntry{
		{Name: "1", Path: "/x/1", Leased: true, LeaseHolder: "h"},
		{Name: "2", Path: "/x/2", OwnerPID: 4242, OwnerStartedAt: 99},
	}}
	if err := WriteState(poolDir, s); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState(poolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Worktrees) != 2 {
		t.Fatalf("round trip got %d worktrees, want 2", len(got.Worktrees))
	}
	if !got.Worktrees[0].Leased || got.Worktrees[0].LeaseHolder != "h" {
		t.Errorf("lease fields lost: %+v", got.Worktrees[0])
	}
	if got.Worktrees[1].OwnerPID != 4242 {
		t.Errorf("owner fields lost: %+v", got.Worktrees[1])
	}
}

func TestReadStateMissingIsEmpty(t *testing.T) {
	got, err := ReadState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Worktrees) != 0 {
		t.Errorf("missing state file should decode to an empty pool, got %d", len(got.Worktrees))
	}
}

// TestRecoverCorruptState is the crux of the state-recovery safety guarantee:
// a truncated/corrupt state file must NOT brick the pool, and every worktree
// rebuilt from disk must be marked leased so it is never silently handed out,
// pruned, or bulk-destroyed before a human verifies it.
func TestRecoverCorruptState(t *testing.T) {
	poolDir := t.TempDir()

	// Simulate a worktree left on disk: poolDir/1/proj/.git (a gitdir pointer).
	wtDir := filepath.Join(poolDir, "1", "proj")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte("gitdir: /somewhere/else\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a truncated/corrupt state file.
	if err := os.WriteFile(stateFilePath(poolDir), []byte("{ this is not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadState(poolDir)
	if err != nil {
		t.Fatalf("ReadState should recover, not fail: %v", err)
	}
	if len(got.Worktrees) != 1 {
		t.Fatalf("recovered %d worktrees, want 1", len(got.Worktrees))
	}
	if !got.Worktrees[0].Leased {
		t.Error("recovered entry must be marked leased until verified")
	}
	if got.Worktrees[0].LeaseHolder != recoveredLeaseHolder {
		t.Errorf("recovered lease holder = %q, want the recovery marker", got.Worktrees[0].LeaseHolder)
	}
}
