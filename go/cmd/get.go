package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zottiben/ai-worktree/go/internal/config"
	"github.com/zottiben/ai-worktree/go/internal/git"
	"github.com/zottiben/ai-worktree/go/internal/pool"
	"github.com/zottiben/ai-worktree/go/internal/process"
	"github.com/zottiben/ai-worktree/go/internal/shell"
	"github.com/zottiben/ai-worktree/go/internal/ui"
)

var (
	getLease       bool
	getLeaseHolder string
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Acquire a worktree from the pool and open a subshell",
	Long: `Acquire a worktree from the pool and open a subshell in it.

Pass --lease for a non-interactive, durable acquire: awt reserves the
worktree, marks it leased in its persistent state, and prints only the worktree's
absolute path to stdout (all banners go to stderr). A leased worktree is never
handed out by a later get and never removed by prune, even with no process
running inside it, until you release it with 'awt return <path>'.`,
	RunE: getRunE,
}

func init() {
	getCmd.Flags().BoolVar(&getLease, "lease", false, "Durably lease a worktree without opening a subshell; print only its path to stdout")
	getCmd.Flags().StringVar(&getLeaseHolder, "lease-holder", "", "Optional label recorded as the lease holder (defaults to $AWT_LEASE_HOLDER)")
	rootCmd.AddCommand(getCmd)
}

func getRunE(cmd *cobra.Command, args []string) error {
	repoRoot, err := git.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	poolDir, err := config.ResolvePoolDir(repoRoot, cfg.Root)
	if err != nil {
		return fmt.Errorf("failed to resolve pool directory: %w", err)
	}

	if err := config.EnsureGitignore(filepath.Dir(poolDir)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update .gitignore: %v\n", err)
	}

	if getLease {
		return getLeaseRunE(repoRoot, poolDir, cfg)
	}

	wtPath, err := pool.Acquire(repoRoot, poolDir, cfg.MaxTrees, cfg.Hooks.PostCreate)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "🌳 Entered worktree at %s. Type 'exit' to return.\n", ui.PrettyPath(wtPath))

	env := []string{
		"AWT_DIR=" + wtPath,
	}
	_, err = shell.Spawn(wtPath, env)

	// Subshell exited — handle return
	if err := git.DetachWorktree(wtPath); err != nil {
		fmt.Fprintf(os.Stderr, "🌳 Warning: failed to detach worktree HEAD: %v\n", err)
	}

	dirty, _ := git.IsDirty(wtPath)
	if dirty {
		fmt.Fprintf(os.Stderr, "🌳 Worktree has uncommitted changes.\n")

		ok, promptErr := ui.Confirm("Clean worktree and return to pool?", true)
		if promptErr != nil || !ok {
			fmt.Fprintln(os.Stderr, "🌳 Worktree left dirty. Use 'awt return --force' to clean it later.")
			return nil
		}
	}

	killLingeringProcesses(wtPath)

	if err := pool.Release(poolDir, wtPath); err != nil {
		fmt.Fprintf(os.Stderr, "🌳 Warning: failed to clean worktree: %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "🌳 Worktree returned to pool.")
	}

	return nil
}

// getLeaseRunE performs a non-interactive, durable acquire. It reserves a
// worktree, marks it leased in persistent state, prints only the worktree path
// to stdout, and routes every human-facing message to stderr so that
// `path=$(awt get --lease)` works cleanly in scripts.
func getLeaseRunE(repoRoot, poolDir string, cfg config.Config) error {
	holder := getLeaseHolder
	if holder == "" {
		holder = os.Getenv("AWT_LEASE_HOLDER")
	}

	wtPath, err := pool.AcquireLease(repoRoot, poolDir, cfg.MaxTrees, cfg.Hooks.PostCreate, holder)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "🌳 Leased worktree at %s. Run 'awt return %s' to release it.\n",
		ui.PrettyPath(wtPath), ui.PrettyPath(wtPath))
	// The bare path is the only thing on stdout, so callers can capture it.
	fmt.Fprintln(os.Stdout, wtPath)
	return nil
}

// killLingeringProcesses terminates any process whose cwd is within the given
// worktree. Called before returning a worktree to the pool so detached tools
// (e.g. agent servers that ignore SIGHUP) don't keep holding the worktree.
func killLingeringProcesses(wtPath string) {
	killed, err := process.TerminateWorktreeProcesses(wtPath, 2*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "🌳 Warning: failed to scan for lingering processes: %v\n", err)
		return
	}
	if len(killed) == 0 {
		return
	}
	names := make([]string, len(killed))
	for i, p := range killed {
		names[i] = p.String()
	}
	fmt.Fprintf(os.Stderr, "🌳 Terminated lingering processes: %s\n", strings.Join(names, ", "))
}
