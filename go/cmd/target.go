package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zottiben/ai-worktree/go/internal/config"
	"github.com/zottiben/ai-worktree/go/internal/git"
	"github.com/zottiben/ai-worktree/go/internal/pool"
)

var errWorktreeUnmanaged = errors.New("worktree unmanaged")

// resolveWorktreeTarget accepts a worktree name or an exact worktree path. Only a
// bare, nonexistent path can be a name: a typo in an explicit path or an
// existing local directory must never silently select a different worktree by
// name.
func resolveWorktreeTarget(target string) (string, string, error) {
	path, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}
	bare := target != "." && target != ".." && filepath.Base(target) == target && filepath.VolumeName(target) == ""
	if bare {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return resolveWorktreeByName(target)
		} else if err != nil {
			return "", "", err
		}
	}
	poolDir, wtPath, err := resolveManagedWorktree(path)
	if !errors.Is(err, errWorktreeUnmanaged) {
		return poolDir, wtPath, err
	}
	if bare {
		if _, named, nameErr := resolveWorktreeByName(target); nameErr == nil {
			return "", "", fmt.Errorf("worktree %s is not managed by awt; %q is an existing path here, so pass worktree %s by its path: %s", path, target, target, named)
		}
	}
	return "", "", fmt.Errorf("worktree %s is not managed by awt", path)
}

func resolveWorktreeByName(name string) (string, string, error) {
	// Resolve through the owning checkout: relative config roots and the pool
	// hash of a local-only repository must not change inside a linked worktree.
	repoRoot, err := git.FindMainRepoRootFrom(".")
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve worktree name %q outside a git repository; specify a worktree path", name)
	}
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return "", "", fmt.Errorf("failed to load config: %w", err)
	}
	poolDir, err := config.ResolvePoolDir(repoRoot, cfg.Root)
	if err != nil {
		return "", "", err
	}
	state, err := pool.ReadState(poolDir)
	if err != nil {
		return "", "", err
	}
	var wtPath string
	for _, wt := range state.Worktrees {
		if wt.Name != name {
			continue
		}
		if wtPath != "" {
			return "", "", fmt.Errorf("more than one worktree named %q in pool %s; specify the exact worktree path", name, poolDir)
		}
		wtPath = wt.Path
	}
	if wtPath == "" {
		return "", "", fmt.Errorf("no worktree named %q in pool %s; run 'awt status' for details", name, poolDir)
	}
	return poolDir, wtPath, nil
}

func resolveManagedWorktree(path string) (string, string, error) {
	poolDir := filepath.Dir(filepath.Dir(path))
	entry, err := pool.FindByPath(poolDir, path)
	if err != nil {
		return "", "", err
	}
	if entry != nil {
		return poolDir, entry.Path, nil
	}

	repoRoot, err := git.FindMainRepoRootFrom(path)
	if err != nil {
		return "", "", errWorktreeUnmanaged
	}
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return "", "", fmt.Errorf("failed to load config: %w", err)
	}
	poolDir, err = config.ResolvePoolDir(repoRoot, cfg.Root)
	if err != nil {
		return "", "", err
	}
	entry, err = pool.FindByPath(poolDir, path)
	if err != nil {
		return "", "", err
	}
	if entry == nil {
		return "", "", errWorktreeUnmanaged
	}
	return poolDir, entry.Path, nil
}
