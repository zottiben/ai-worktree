package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/zottiben/ai-worktree/go/internal/config"
	"github.com/zottiben/ai-worktree/go/internal/git"
	"github.com/zottiben/ai-worktree/go/internal/pool"
	"github.com/zottiben/ai-worktree/go/internal/ui"
)

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of all worktrees in the pool",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := git.FindMainRepoRootFrom(".")
		if err != nil {
			return fmt.Errorf("not in a git repository: %w", err)
		}

		cfg, err := config.Load(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		poolDir, err := config.ResolvePoolDir(repoRoot, cfg.Root)
		if err != nil {
			return err
		}

		worktrees, err := pool.List(repoRoot, poolDir)
		if err != nil {
			return err
		}

		if statusJSON {
			return writeJSON(os.Stdout, statusToJSON(poolDir, worktrees))
		}

		if len(worktrees) == 0 {
			fmt.Fprintln(os.Stderr, "🌳 No worktrees in pool.")
			return nil
		}

		green := color.New(color.FgGreen).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()
		yellow := color.New(color.FgYellow).SprintFunc()
		cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
		magenta := color.New(color.FgMagenta).SprintFunc()

		// statusWidth must be >= longest status string ("you're here" = 11)
		const statusWidth = 11
		nameWidth := 4
		missing := false
		for _, wt := range worktrees {
			nameWidth = max(nameWidth, len(wt.Name))
			missing = missing || wt.Status == pool.StatusMissing
		}

		for _, wt := range worktrees {
			var status string
			switch wt.Status {
			case pool.StatusAvailable:
				status = green(wt.Status)
			case pool.StatusInUse:
				status = red(wt.Status)
			case pool.StatusDirty, pool.StatusMissing:
				status = yellow(wt.Status)
			case pool.StatusLeased:
				status = magenta(wt.Status)
			case pool.StatusHere:
				status = cyan(wt.Status)
			}

			statusPad := strings.Repeat(" ", statusWidth-len(wt.Status))
			line := fmt.Sprintf("%-*s  %s%s  %s", nameWidth, wt.Name, status, statusPad, ui.PrettyPath(wt.Path))
			if wt.Recovered && wt.Status == pool.StatusMissing {
				line += "  (recovered)"
			} else if wt.LeaseHolder != "" {
				line += fmt.Sprintf("  (held by %s)", wt.LeaseHolder)
			}
			fmt.Fprintln(os.Stdout, line)

			if len(wt.Processes) > 0 {
				var procStrs []string
				for _, p := range wt.Processes {
					procStrs = append(procStrs, p.String())
				}
				fmt.Fprintf(os.Stdout, "%s%s\n", strings.Repeat(" ", nameWidth+2+statusWidth+2), strings.Join(procStrs, ", "))
			}
		}
		if missing {
			fmt.Fprintln(os.Stdout, "🌳 Preview missing checkout cleanup with 'awt prune --missing'.")
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Emit machine-readable JSON to stdout")
	rootCmd.AddCommand(statusCmd)
}
