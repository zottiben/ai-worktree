package pool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRun runs a git command in dir and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// newTestRepo creates a throwaway git repo with a single commit on main.
func newTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitRun(t, repo, "-c", "init.defaultBranch=main", "init", "-q")
	gitRun(t, repo, "config", "user.email", "t@test.local")
	gitRun(t, repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "init")
	return repo
}

func newPoolDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "pool")
}

func TestAcquireCreatesWorktree(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatalf("worktree .git missing: %v", err)
	}

	state, err := ReadState(poolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Worktrees) != 1 {
		t.Fatalf("state has %d worktrees, want 1", len(state.Worktrees))
	}
	e := state.Worktrees[0]
	if e.Leased {
		t.Error("interactive acquire must not set a durable lease")
	}
	if e.OwnerPID == 0 {
		t.Error("interactive acquire must set an owner reservation")
	}
}

func TestReleaseThenReacquireReuses(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt1, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(poolDir, wt1); err != nil {
		t.Fatalf("Release: %v", err)
	}
	wt2, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wt1 != wt2 {
		t.Errorf("expected reuse of %s after release, got new worktree %s", wt1, wt2)
	}
	state, _ := ReadState(poolDir)
	if len(state.Worktrees) != 1 {
		t.Errorf("reuse should keep pool at 1, got %d", len(state.Worktrees))
	}
}

func TestAcquireLeaseSetsDurableLease(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	if _, err := AcquireLease(repo, poolDir, 4, nil, "holder-x"); err != nil {
		t.Fatal(err)
	}
	state, _ := ReadState(poolDir)
	e := state.Worktrees[0]
	if !e.Leased {
		t.Error("AcquireLease must set Leased")
	}
	if e.LeaseHolder != "holder-x" {
		t.Errorf("LeaseHolder = %q, want holder-x", e.LeaseHolder)
	}
	if e.OwnerPID != 0 {
		t.Error("a lease is process-independent and must carry no owner reservation")
	}
}

func TestLeasedWorktreeNeverReacquired(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt1, err := AcquireLease(repo, poolDir, 4, nil, "h")
	if err != nil {
		t.Fatal(err)
	}
	wt2, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wt1 == wt2 {
		t.Error("a leased worktree must never be handed out by a later acquire")
	}
	state, _ := ReadState(poolDir)
	if len(state.Worktrees) != 2 {
		t.Errorf("pool should have grown to 2, got %d", len(state.Worktrees))
	}
}

func TestReleaseClearsLease(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := AcquireLease(repo, poolDir, 4, nil, "h")
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(poolDir, wt); err != nil {
		t.Fatal(err)
	}
	state, _ := ReadState(poolDir)
	if state.Worktrees[0].Leased {
		t.Error("Release must clear the durable lease")
	}
}

func TestListReportsLeasedWithHolder(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	if _, err := AcquireLease(repo, poolDir, 4, nil, "holder-y"); err != nil {
		t.Fatal(err)
	}
	list, err := List(poolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d, want 1", len(list))
	}
	if list[0].Status != StatusLeased {
		t.Errorf("status = %q, want %q", list[0].Status, StatusLeased)
	}
	if list[0].LeaseHolder != "holder-y" {
		t.Errorf("LeaseHolder = %q, want holder-y", list[0].LeaseHolder)
	}
}

// orphanOwner rewrites the sole worktree's owner reservation to look like a
// dead process (a pid whose recorded start time can never match) that was
// recorded under bootTime, simulating a reservation left behind by a prior run.
func orphanOwner(t *testing.T, poolDir string, bootTime uint64) {
	t.Helper()
	state, err := ReadState(poolDir)
	if err != nil {
		t.Fatal(err)
	}
	state.Worktrees[0].OwnerPID = 999999
	state.Worktrees[0].OwnerStartedAt = 1
	state.Worktrees[0].OwnerBootTime = bootTime
	if err := WriteState(poolDir, state); err != nil {
		t.Fatal(err)
	}
}

func TestRebootPreservesInUseWorktreeAsLease(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The owner reservation was recorded under boot time 1000; the machine has
	// since rebooted (now 2000) and the owning process is gone.
	orphanOwner(t, poolDir, 1000)
	currentBootTime = func() (uint64, bool) { return 2000, true }
	t.Cleanup(func() { currentBootTime = defaultBootTime })

	list, err := List(poolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != StatusLeased {
		t.Fatalf("after reboot want one leased worktree, got %+v", list)
	}

	// A later acquire must not reset or hand out the preserved worktree.
	wt2, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wt2 == wt {
		t.Error("a worktree preserved across a reboot must never be reacquired")
	}
}

func TestCrashReclaimsInUseWorktree(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	wt, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Same boot session (1000) but the owning process died: a genuine crash,
	// so the worktree should be reclaimed, not preserved.
	orphanOwner(t, poolDir, 1000)
	currentBootTime = func() (uint64, bool) { return 1000, true }
	t.Cleanup(func() { currentBootTime = defaultBootTime })

	state := healState(mustReadState(t, poolDir))
	if state.Worktrees[0].Leased {
		t.Fatal("a crashed owner must not leave the worktree leased")
	}

	wt2, err := Acquire(repo, poolDir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wt2 != wt {
		t.Errorf("a crash-reclaimed worktree should be reused, got new %s", wt2)
	}
}

func mustReadState(t *testing.T, poolDir string) State {
	t.Helper()
	state, err := ReadState(poolDir)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestPoolSizeLimitExhausted(t *testing.T) {
	repo := newTestRepo(t)
	poolDir := newPoolDir(t)

	// Lease the only allowed worktree, then a second acquire cannot reuse it
	// (leased) nor create another (max_trees = 1).
	if _, err := AcquireLease(repo, poolDir, 1, nil, "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(repo, poolDir, 1, nil); err == nil {
		t.Error("expected pool-exhausted error when the only worktree is leased")
	}
}
