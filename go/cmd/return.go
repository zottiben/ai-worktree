package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zottiben/ai-worktree/go/internal/git"
	"github.com/zottiben/ai-worktree/go/internal/pool"
	"github.com/zottiben/ai-worktree/go/internal/ui"
)

var returnForce bool

var returnCmd = &cobra.Command{
	Use:   "return [name|path]",
	Short: "Terminate lingering processes and return a worktree",
	Long: `Release a worktree's lease or reservation, clean it, and return it to the
pool.

  awt return <name>            Return a worktree in the current repo's pool.
  awt return <worktree-path>   Return exactly one worktree by path.
  awt return                   Return the worktree that contains the current
                               directory ($AWT_DIR inside an awt subshell).

Names are those shown by 'awt status', e.g. 'awt return 3'. Existing relative
paths are treated as paths, not names; use ./path to explicitly name a path.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		poolDir, wtPath, err := resolveReturnTarget(args)
		if err != nil {
			return err
		}

		if !returnForce {
			dirty, _ := git.IsDirty(wtPath)
			if dirty {
				ok, err := ui.Confirm("Worktree has uncommitted changes. Clean and return?", true)
				if err != nil || !ok {
					fmt.Fprintln(os.Stderr, "🌳 Aborted.")
					return nil
				}
			}

			if err := git.DetachWorktree(wtPath); err != nil {
				return fmt.Errorf("failed to detach worktree HEAD: %w", err)
			}
		}

		killLingeringProcesses(wtPath)

		if err := pool.Release(poolDir, wtPath); err != nil {
			return fmt.Errorf("failed to return worktree: %w", err)
		}

		fmt.Fprintln(os.Stderr, "🌳 Worktree returned to pool.")
		return nil
	},
}

func init() {
	returnCmd.Flags().BoolVar(&returnForce, "force", false, "Clean, reset, and return without prompting")
	rootCmd.AddCommand(returnCmd)
}

func resolveReturnTarget(args []string) (string, string, error) {
	if len(args) > 0 {
		return resolveWorktreeTarget(args[0])
	}
	if env := os.Getenv("AWT_DIR"); env != "" {
		path, err := filepath.Abs(env)
		if err != nil {
			return "", "", err
		}
		poolDir, wtPath, err := resolveManagedWorktree(path)
		if errors.Is(err, errWorktreeUnmanaged) {
			return "", "", fmt.Errorf("AWT_DIR worktree %s is not managed by awt", path)
		}
		return poolDir, wtPath, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root, err := git.FindRepoRootFrom(cwd)
	if err != nil {
		return "", "", fmt.Errorf("not in a git repository: %w", err)
	}
	poolDir, wtPath, err := resolveManagedWorktree(root)
	if errors.Is(err, errWorktreeUnmanaged) {
		return "", "", fmt.Errorf("%s is not inside a worktree managed by awt; pass a worktree name from 'awt status'", cwd)
	}
	return poolDir, wtPath, err
}
