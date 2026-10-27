package pool

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/zottiben/ai-worktree/go/internal/git"
)

const unrecordedLeaseHolder = "recovered: registered with Git but absent from pool state; verify before reuse"

func reconcileState(repoRoot, poolDir string, state State) (State, error) {
	paths, err := git.ListLinkedWorktreePaths(repoRoot)
	if err != nil {
		return State{}, fmt.Errorf("read registered worktrees: %w", err)
	}
	root, err := resolvePath(poolDir)
	if err != nil {
		return State{}, err
	}
	known := make(map[string]bool, len(state.Worktrees))
	for _, wt := range state.Worktrees {
		path, err := resolvePath(wt.Path)
		if err != nil {
			return State{}, err
		}
		known[path] = true
	}
	for _, path := range paths {
		resolved, err := resolvePath(path)
		if err != nil {
			return State{}, err
		}
		if known[resolved] || filepath.Dir(filepath.Dir(resolved)) != root || filepath.Base(resolved) != filepath.Base(repoRoot) {
			continue
		}
		name := filepath.Base(filepath.Dir(resolved))
		now := time.Now()
		state.Worktrees = append(state.Worktrees, WorktreeEntry{
			Name:        name,
			Path:        filepath.Join(poolDir, name, filepath.Base(repoRoot)),
			CreatedAt:   now,
			Leased:      true,
			LeaseHolder: unrecordedLeaseHolder,
			LeasedAt:    now,
		})
		known[resolved] = true
	}
	sort.SliceStable(state.Worktrees, func(i, j int) bool {
		return lessByWorktreeName(state.Worktrees[i].Name, state.Worktrees[j].Name)
	})
	return healState(state), nil
}
