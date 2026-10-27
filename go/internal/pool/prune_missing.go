package pool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zottiben/ai-worktree/go/internal/git"
	"github.com/zottiben/ai-worktree/go/internal/process"
)

const PruneSkipMissing = "missing checkout"

func PruneMissing(repoRoot, poolDir string, dryRun bool) (PruneResult, error) {
	var result PruneResult
	err := WithStateLock(poolDir, func() error {
		state, err := ReadState(poolDir)
		if err != nil {
			return err
		}
		state, err = reconcileState(repoRoot, poolDir, state)
		if err != nil {
			return err
		}
		registered, err := git.ListLinkedWorktrees(repoRoot)
		if err != nil {
			return err
		}
		byPath := make(map[string]git.RegisteredWorktree, len(registered))
		for _, wt := range registered {
			path, err := resolvePath(wt.Path)
			if err != nil {
				return err
			}
			byPath[path] = wt
		}
		root, err := resolvePath(poolDir)
		if err != nil {
			return err
		}
		var kept []WorktreeEntry
		for _, wt := range state.Worktrees {
			kept = append(kept, wt)
			skip := func(reason, detail string) {
				result.Skipped = append(result.Skipped, newPruneSkipped(wt.Name, wt.Path, PruneSkipMissing, reason, detail))
			}
			if info, err := os.Lstat(wt.Path); err == nil {
				if _, gitErr := os.Stat(filepath.Join(wt.Path, ".git")); gitErr != nil || info.Mode()&os.ModeSymlink != 0 {
					skip("checkout path still exists; no files will be deleted", "")
				}
				continue
			} else if !os.IsNotExist(err) {
				skip("cannot inspect checkout", err.Error())
				continue
			}
			path, err := resolvePath(wt.Path)
			if err != nil {
				skip("cannot resolve checkout path", err.Error())
				continue
			}
			if filepath.Dir(filepath.Dir(path)) != root || filepath.Base(path) != filepath.Base(repoRoot) {
				skip("outside the current repository pool", "")
				continue
			}
			if wt.Destroying || ownerAlive(wt) {
				skip("reserved or in use", "")
				continue
			}
			if wt.Leased && !recoveryLease(wt) {
				skip("explicit lease protected", wt.LeaseHolder)
				continue
			}
			if inUse, err := process.IsWorktreeInUse(path); err != nil || inUse {
				detail := ""
				if err != nil {
					detail = err.Error()
				}
				skip("in use or cannot check processes", detail)
				continue
			}
			registration, exists := byPath[path]
			candidate := PruneWorktree{Name: wt.Name, Path: wt.Path}
			if exists {
				if registration.Locked || !registration.Prunable {
					skip("Git registration is locked or not prunable", "")
					continue
				}
				candidate.Warning = fmt.Sprintf("HEAD recovery ref: %s", registration.RecoveryRef())
			} else {
				candidate.Warning = "state entry only; Git registration already absent"
			}
			result.Candidates = append(result.Candidates, candidate)
			if dryRun {
				continue
			}
			if exists {
				if err := git.RemoveMissingWorktree(repoRoot, registration); err != nil {
					skip("could not preserve HEAD or remove registration", err.Error())
					continue
				}
			}
			kept = kept[:len(kept)-1]
			result.Pruned = append(result.Pruned, candidate)
		}
		if dryRun {
			return nil
		}
		state.Worktrees = kept
		return WriteState(poolDir, state)
	})
	return result, err
}
