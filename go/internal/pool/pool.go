package pool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zottiben/ai-worktree/go/internal/git"
	"github.com/zottiben/ai-worktree/go/internal/hooks"
	"github.com/zottiben/ai-worktree/go/internal/process"
)

// Worktree status labels reported by List.
const (
	StatusAvailable = "available"
	StatusDirty     = "dirty"
	StatusInUse     = "in-use"
	StatusLeased    = "leased"
	StatusHere      = "you're here"
)

// WorktreeStatus describes one managed worktree as reported by List.
type WorktreeStatus struct {
	Name      string
	Path      string
	Status    string
	Processes []process.ProcessInfo
	// LeaseHolder is the recorded holder for a leased worktree, if any.
	LeaseHolder string
}

// acquireOptions controls how Acquire reserves the worktree it hands out.
type acquireOptions struct {
	// lease records a durable, process-independent reservation instead of the
	// default short-lived owner reservation.
	lease bool
	// leaseHolder is an optional label stored with a lease.
	leaseHolder string
	// hookStdout/hookStderr receive post-create hook output. Lease mode routes
	// hook stdout to stderr so the worktree path stays the only stdout line.
	hookStdout io.Writer
	hookStderr io.Writer
}

// Acquire reserves a clean worktree from the pool with a short-lived owner
// reservation (the calling process). It is the backing call for the interactive
// `awt get` subshell.
func Acquire(repoRoot, poolDir string, poolSize int, postCreate []string) (string, error) {
	return acquire(repoRoot, poolDir, poolSize, postCreate, acquireOptions{
		hookStdout: os.Stdout,
		hookStderr: os.Stderr,
	})
}

// AcquireLease reserves a clean worktree and marks it durably LEASED so the
// reservation survives with zero processes running inside it. The lease persists
// until it is released by Release. holder is an optional label recorded with the
// lease for diagnostics. Post-create hook stdout is routed to stderr so callers
// can capture the returned path as the sole stdout line.
func AcquireLease(repoRoot, poolDir string, poolSize int, postCreate []string, holder string) (string, error) {
	return acquire(repoRoot, poolDir, poolSize, postCreate, acquireOptions{
		lease:       true,
		leaseHolder: holder,
		hookStdout:  os.Stderr,
		hookStderr:  os.Stderr,
	})
}

func acquire(repoRoot, poolDir string, poolSize int, postCreate []string, opts acquireOptions) (string, error) {
	branch, err := git.GetDefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "🌳 Setting up worktree...\n")
	if git.HasRemote(repoRoot, "origin") {
		if err := git.Fetch(repoRoot); err != nil {
			return "", fmt.Errorf("fetch failed: %w", err)
		}
	}

	var acquired string
	var runPostCreate bool

	err = WithStateLock(poolDir, func() error {
		state, err := ReadState(poolDir)
		if err != nil {
			return err
		}

		state = healState(state)

		// Try to find an available worktree (clean, not in-use, not leased)
		for i, wt := range state.Worktrees {
			if wt.Destroying || wt.Leased || ownerAlive(wt) {
				continue
			}
			inUse, _ := process.IsWorktreeInUse(wt.Path)
			if inUse {
				continue
			}
			dirty, _ := git.IsDirty(wt.Path)
			if dirty {
				continue
			}
			// Found an available one — reset it
			if err := git.ResetWorktree(wt.Path, branch); err != nil {
				continue
			}
			if err := markAcquired(&state.Worktrees[i], opts); err != nil {
				return err
			}
			acquired = wt.Path
			if err := WriteState(poolDir, state); err != nil {
				return err
			}
			runPostCreate = true
			return nil
		}

		// No available worktree — create new if pool allows
		if len(state.Worktrees) >= poolSize {
			return fmt.Errorf("all %d worktrees are in use or dirty (max_trees = %d). Run 'awt status' to see details, or increase max_trees in awt.toml", len(state.Worktrees), poolSize)
		}

		name := nextName(state)
		repoName := filepath.Base(repoRoot)
		wtPath := filepath.Join(poolDir, name, repoName)

		if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
			return err
		}

		if err := git.AddWorktree(repoRoot, wtPath, branch); err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}

		entry := WorktreeEntry{
			Name:      name,
			Path:      wtPath,
			CreatedAt: time.Now(),
		}
		if err := markAcquired(&entry, opts); err != nil {
			return err
		}
		state.Worktrees = append(state.Worktrees, entry)

		acquired = wtPath
		if err := WriteState(poolDir, state); err != nil {
			return err
		}
		runPostCreate = true
		return nil
	})
	if err != nil {
		return "", err
	}
	if runPostCreate {
		hooks.Run(postCreate, acquired, opts.hookStdout, opts.hookStderr)
	}

	return acquired, nil
}

// markAcquired stamps an acquired worktree entry: a durable lease in lease mode,
// otherwise the default short-lived owner reservation.
func markAcquired(wt *WorktreeEntry, opts acquireOptions) error {
	if opts.lease {
		wt.Leased = true
		wt.LeaseHolder = opts.leaseHolder
		wt.LeasedAt = time.Now()
		// A lease is process-independent, so it carries no owner reservation.
		wt.OwnerPID = 0
		wt.OwnerStartedAt = 0
		wt.OwnerBootTime = 0
		return nil
	}
	return reserveOwner(wt)
}

// Release resets a managed worktree, clears its short-lived owner reservation or
// durable lease, and returns it to the available pool.
func Release(poolDir, worktreePath string) error {
	repoRoot, err := git.FindRepoRootFrom(worktreePath)
	if err != nil {
		return err
	}
	branch, err := git.GetDefaultBranch(repoRoot)
	if err != nil {
		return err
	}
	if err := WithStateLock(poolDir, func() error {
		state, err := ReadState(poolDir)
		if err != nil {
			return err
		}
		for _, wt := range state.Worktrees {
			if wt.Path == worktreePath && wt.Destroying {
				return fmt.Errorf("worktree %s is being destroyed", worktreePath)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := git.ResetWorktree(worktreePath, branch); err != nil {
		return err
	}
	return WithStateLock(poolDir, func() error {
		state, err := ReadState(poolDir)
		if err != nil {
			return err
		}
		for i := range state.Worktrees {
			if state.Worktrees[i].Path == worktreePath {
				if state.Worktrees[i].Destroying {
					return fmt.Errorf("worktree %s is being destroyed", worktreePath)
				}
				state.Worktrees[i].OwnerPID = 0
				state.Worktrees[i].OwnerStartedAt = 0
				state.Worktrees[i].OwnerBootTime = 0
				clearLease(&state.Worktrees[i])
				break
			}
		}
		return WriteState(poolDir, state)
	})
}

// List returns the current status of managed worktrees in poolDir.
// Leased worktrees are reported with StatusLeased and their optional holder.
func List(poolDir string) ([]WorktreeStatus, error) {
	var result []WorktreeStatus

	err := WithStateLock(poolDir, func() error {
		state, err := ReadState(poolDir)
		if err != nil {
			return err
		}

		state = healState(state)
		if err := WriteState(poolDir, state); err != nil {
			return err
		}

		cwd, _ := os.Getwd()

		for _, wt := range state.Worktrees {
			if wt.Destroying {
				continue
			}
			ws := WorktreeStatus{
				Name:   wt.Name,
				Path:   wt.Path,
				Status: StatusAvailable,
			}

			procs, _ := process.FindProcessesInWorktree(wt.Path)
			ws.Processes = procs

			if wt.Leased {
				ws.Status = StatusLeased
				ws.LeaseHolder = wt.LeaseHolder
			} else if ownerAlive(wt) {
				ws.Status = StatusInUse
			} else if len(procs) > 0 {
				ws.Status = StatusInUse
				if cwdInWorktree(cwd, wt.Path) {
					ws.Status = StatusHere
				}
			} else if dirty, _ := git.IsDirty(wt.Path); dirty {
				ws.Status = StatusDirty
			}

			result = append(result, ws)
		}
		return nil
	})

	return result, err
}

// FindByPath returns the state entry for worktreePath, or nil if the pool does
// not manage it.
func FindByPath(poolDir, path string) (*WorktreeEntry, error) {
	state, err := ReadState(poolDir)
	if err != nil {
		return nil, err
	}
	for _, wt := range state.Worktrees {
		if wt.Path == path {
			return &wt, nil
		}
	}
	return nil, nil
}

// orphanedByRestartHolder labels a worktree whose owner reservation outlived a
// machine restart. healState converts such a reservation into a durable lease
// so the worktree (and any committed-but-unpushed work on it) is preserved:
// resume it with `awt enter <name>` or release it with `awt return <path>`.
const orphanedByRestartHolder = "orphaned: machine restarted while in use; resume with 'awt enter' or release with 'awt return'"

// defaultBootTime is the production source of the system boot time.
var defaultBootTime = process.BootTime

// currentBootTime reports the system boot time. It is a variable so tests can
// simulate a reboot by swapping in a different value.
var currentBootTime = defaultBootTime

// healState drops entries whose directory is gone and reconciles owner
// reservations whose owning process has died (pid reuse aware). A reservation
// that died within the current boot session is a crash and is cleared, freeing
// the worktree. A reservation that outlived a machine restart is preserved by
// converting it to a durable lease, so a reboot never silently resets or hands
// out a worktree that was in use. Durable leases are never cleared here.
func healState(state State) State {
	curBoot, bootOK := currentBootTime()
	var healed []WorktreeEntry
	for _, wt := range state.Worktrees {
		if _, err := os.Stat(wt.Path); err != nil {
			continue
		}
		if wt.OwnerPID != 0 && !ownerAlive(wt) {
			rebooted := rebootedSince(wt, curBoot, bootOK)
			wt.OwnerPID = 0
			wt.OwnerStartedAt = 0
			wt.OwnerBootTime = 0
			wt.Destroying = false
			if rebooted && !wt.Leased {
				wt.Leased = true
				wt.LeaseHolder = orphanedByRestartHolder
				wt.LeasedAt = time.Now()
			}
		}
		healed = append(healed, wt)
	}
	state.Worktrees = healed
	return state
}

// rebootedSince reports whether the machine has rebooted since this owner
// reservation was recorded. A reservation with no recorded boot time predates
// the OwnerBootTime field, or the current boot time is unavailable; either way
// it is treated conservatively as a possible reboot so the worktree is
// preserved rather than silently reset (Hard rule #2: destructive ops fail
// safe).
func rebootedSince(wt WorktreeEntry, curBoot uint64, bootOK bool) bool {
	if wt.OwnerBootTime == 0 || !bootOK {
		return true
	}
	return wt.OwnerBootTime != curBoot
}

func ownerAlive(wt WorktreeEntry) bool {
	if wt.OwnerPID == 0 || wt.OwnerStartedAt == 0 {
		return false
	}
	startedAt, ok := process.StartedAt(wt.OwnerPID)
	return ok && startedAt == wt.OwnerStartedAt
}

func reserveOwner(wt *WorktreeEntry) error {
	pid := int32(os.Getpid())
	startedAt, ok := process.StartedAt(pid)
	if !ok {
		return fmt.Errorf("failed to determine owner process identity")
	}
	wt.OwnerPID = pid
	wt.OwnerStartedAt = startedAt
	if bootTime, ok := currentBootTime(); ok {
		wt.OwnerBootTime = bootTime
	}
	return nil
}

// clearLease removes any durable lease from a worktree entry.
func clearLease(wt *WorktreeEntry) {
	wt.Leased = false
	wt.LeaseHolder = ""
	wt.LeasedAt = time.Time{}
}

func sameDestroyReservation(current, reserved WorktreeEntry) bool {
	return current.Path == reserved.Path &&
		current.Destroying &&
		current.OwnerPID == reserved.OwnerPID &&
		current.OwnerStartedAt == reserved.OwnerStartedAt
}

func cwdInWorktree(cwd, worktreePath string) bool {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return false
	}
	absWt, err := filepath.Abs(worktreePath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absWt, absCwd)
	if err != nil {
		return false
	}
	return rel == "." || !filepath.IsAbs(rel) && len(rel) >= 1 && rel[0] != '.'
}

func nextName(state State) string {
	max := 0
	for _, wt := range state.Worktrees {
		if n, err := strconv.Atoi(wt.Name); err == nil && n > max {
			max = n
		}
	}
	return strconv.Itoa(max + 1)
}
