package cmd

import (
	"encoding/json"
	"io"

	"github.com/zottiben/ai-worktree/go/internal/pool"
)

// This file defines the stable machine-readable JSON shapes emitted by the
// --json flags. They are the contract consumed by the TypeScript SDK, so field
// names are camelCase and every slice is emitted as [] (never null).

type jsonProcess struct {
	PID  int32  `json:"pid"`
	Name string `json:"name"`
}

type jsonWorktreeStatus struct {
	Name        string        `json:"name"`
	Path        string        `json:"path"`
	Status      string        `json:"status"`
	LeaseHolder string        `json:"leaseHolder"`
	Processes   []jsonProcess `json:"processes"`
}

type jsonStatusResult struct {
	PoolDir   string               `json:"poolDir"`
	Worktrees []jsonWorktreeStatus `json:"worktrees"`
}

type jsonPruneWorktree struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Orphaned bool   `json:"orphaned"`
	Warning  string `json:"warning"`
}

type jsonPruneSkipped struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail"`
}

type jsonPruneResult struct {
	DryRun           bool                `json:"dryRun"`
	Candidates       []jsonPruneWorktree `json:"candidates"`
	Pruned           []jsonPruneWorktree `json:"pruned"`
	Skipped          []jsonPruneSkipped  `json:"skipped"`
	ReclaimableBytes int64               `json:"reclaimableBytes"`
	FreedBytes       int64               `json:"freedBytes"`
	PoolCount        *int                `json:"poolCount,omitempty"`
}

type jsonDestroyTarget struct {
	Name      string        `json:"name"`
	Path      string        `json:"path"`
	Classes   []string      `json:"classes"`
	Bytes     int64         `json:"bytes"`
	Detail    string        `json:"detail"`
	Processes []jsonProcess `json:"processes"`
}

type jsonDestroySkip struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Classes     []string `json:"classes"`
	NeededFlags []string `json:"neededFlags"`
	LeasedBulk  bool     `json:"leasedBulk"`
	Detail      string   `json:"detail"`
}

type jsonDestroyResult struct {
	DryRun       bool                `json:"dryRun"`
	Planned      []jsonDestroyTarget `json:"planned"`
	Destroyed    []jsonDestroyTarget `json:"destroyed"`
	Skipped      []jsonDestroySkip   `json:"skipped"`
	PlannedBytes int64               `json:"plannedBytes"`
	FreedBytes   int64               `json:"freedBytes"`
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func statusToJSON(poolDir string, worktrees []pool.WorktreeStatus) jsonStatusResult {
	out := jsonStatusResult{PoolDir: poolDir, Worktrees: make([]jsonWorktreeStatus, 0, len(worktrees))}
	for _, wt := range worktrees {
		procs := make([]jsonProcess, 0, len(wt.Processes))
		for _, p := range wt.Processes {
			procs = append(procs, jsonProcess{PID: p.PID, Name: p.Name})
		}
		out.Worktrees = append(out.Worktrees, jsonWorktreeStatus{
			Name:        wt.Name,
			Path:        wt.Path,
			Status:      wt.Status,
			LeaseHolder: wt.LeaseHolder,
			Processes:   procs,
		})
	}
	return out
}

func pruneWorktreesToJSON(worktrees []pool.PruneWorktree) []jsonPruneWorktree {
	out := make([]jsonPruneWorktree, 0, len(worktrees))
	for _, wt := range worktrees {
		out = append(out, jsonPruneWorktree{
			Name:     wt.Name,
			Path:     wt.Path,
			Bytes:    wt.Bytes,
			Orphaned: wt.Orphaned,
			Warning:  wt.Warning,
		})
	}
	return out
}

func pruneSkippedToJSON(skipped []pool.PruneSkipped) []jsonPruneSkipped {
	out := make([]jsonPruneSkipped, 0, len(skipped))
	for _, wt := range skipped {
		out = append(out, jsonPruneSkipped{
			Name:     wt.Name,
			Path:     wt.Path,
			Category: pruneSkipCategory(wt),
			Reason:   wt.Reason,
			Detail:   wt.Detail,
		})
	}
	return out
}

func pruneResultToJSON(result pool.PruneResult, dryRun bool) jsonPruneResult {
	return jsonPruneResult{
		DryRun:           dryRun,
		Candidates:       pruneWorktreesToJSON(result.Candidates),
		Pruned:           pruneWorktreesToJSON(result.Pruned),
		Skipped:          pruneSkippedToJSON(result.Skipped),
		ReclaimableBytes: result.ReclaimableBytes,
		FreedBytes:       result.FreedBytes,
	}
}

func destroyClassesToStrings(t pool.DestroyTarget) []string {
	classes := t.Classes
	if len(classes) == 0 && t.Class != "" {
		classes = []pool.DestroyClass{t.Class}
	}
	out := make([]string, 0, len(classes))
	for _, c := range classes {
		out = append(out, string(c))
	}
	return out
}

func destroyTargetsToJSON(targets []pool.DestroyTarget) []jsonDestroyTarget {
	out := make([]jsonDestroyTarget, 0, len(targets))
	for _, t := range targets {
		procs := make([]jsonProcess, 0, len(t.Processes))
		for _, p := range t.Processes {
			procs = append(procs, jsonProcess{PID: p.PID, Name: p.Name})
		}
		out = append(out, jsonDestroyTarget{
			Name:      t.Name,
			Path:      t.Path,
			Classes:   destroyClassesToStrings(t),
			Bytes:     t.Bytes,
			Detail:    t.Detail,
			Processes: procs,
		})
	}
	return out
}

func destroySkipsToJSON(skips []pool.DestroySkip) []jsonDestroySkip {
	out := make([]jsonDestroySkip, 0, len(skips))
	for _, s := range skips {
		needed := s.NeededFlags
		if len(needed) == 0 && s.NeededFlag != "" {
			needed = []string{s.NeededFlag}
		}
		if needed == nil {
			needed = []string{}
		}
		out = append(out, jsonDestroySkip{
			Name:        s.Target.Name,
			Path:        s.Target.Path,
			Classes:     destroyClassesToStrings(s.Target),
			NeededFlags: needed,
			LeasedBulk:  s.LeasedBulk,
			Detail:      s.Target.Detail,
		})
	}
	return out
}

func destroyResultToJSON(result pool.DestroyResult, dryRun bool) jsonDestroyResult {
	return jsonDestroyResult{
		DryRun:       dryRun,
		Planned:      destroyTargetsToJSON(result.Planned),
		Destroyed:    destroyTargetsToJSON(result.Destroyed),
		Skipped:      destroySkipsToJSON(result.Skipped),
		PlannedBytes: result.PlannedBytes,
		FreedBytes:   result.FreedBytes,
	}
}
