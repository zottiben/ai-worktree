package git

import (
	"fmt"
	"os"
)

func RemoveMissingWorktree(repoRoot string, expected RegisteredWorktree) error {
	worktrees, err := ListLinkedWorktrees(repoRoot)
	if err != nil {
		return err
	}
	var current *RegisteredWorktree
	for i := range worktrees {
		if worktrees[i].Path == expected.Path {
			current = &worktrees[i]
			break
		}
	}
	if current == nil || *current != expected {
		return fmt.Errorf("worktree registration changed; preview cleanup again")
	}
	if current.Locked || !current.Prunable {
		return fmt.Errorf("Git has not marked this worktree prunable or it is locked")
	}
	if _, err := os.Lstat(current.Path); err == nil {
		return fmt.Errorf("worktree path exists; refusing missing-worktree cleanup")
	} else if !os.IsNotExist(err) {
		return err
	}
	commit, err := refCommit(repoRoot, current.Head)
	if err != nil {
		return fmt.Errorf("cannot preserve missing worktree HEAD: %w", err)
	}
	ref := current.RecoveryRef()
	old := ""
	if refExists(repoRoot, ref) {
		old = commit
	}
	if _, err := runGit(repoRoot, "update-ref", "--no-deref", ref, commit, old); err != nil {
		return fmt.Errorf("cannot preserve HEAD at %s: %w", ref, err)
	}
	if _, err := os.Lstat(current.Path); err == nil {
		return fmt.Errorf("worktree path reappeared; refusing missing-worktree cleanup")
	} else if !os.IsNotExist(err) {
		return err
	}
	return RemoveCleanWorktree(repoRoot, current.Path)
}
