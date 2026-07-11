/**
 * TypeScript types for the `awt` CLI contract.
 *
 * The interfaces prefixed with a JSON shape (Status/Prune/Destroy) mirror,
 * field-for-field, the JSON documents emitted by the Go binary when invoked
 * with `--json`. Keep them in exact sync with the binary — they are a wire
 * contract, not an internal convenience type.
 */

/** A live process detected inside a worktree. */
export interface ProcessInfo {
  /** OS process id. */
  pid: number;
  /** Best-effort process name/command. */
  name: string;
}

/** Reservation/health state of a single worktree slot. */
export type WorktreeState =
  | "available"
  | "in-use"
  | "dirty"
  | "leased"
  | "you're here";

/** One worktree slot, as reported by `awt status --json`. */
export interface WorktreeStatus {
  /** Numeric slot name, e.g. `"1"`. */
  name: string;
  /** Absolute path to the worktree. */
  path: string;
  /** Current reservation/health state. */
  status: WorktreeState;
  /** Durable-lease holder label, or `""` when not leased. */
  leaseHolder: string;
  /** Processes currently running inside the worktree. */
  processes: ProcessInfo[];
}

/** Full result of `awt status --json`. */
export interface StatusResult {
  /** Absolute path to the pool directory holding the worktrees. */
  poolDir: string;
  /** Every managed worktree in the pool. */
  worktrees: WorktreeStatus[];
}

/** A worktree eligible for (or that was) pruned — from `awt prune --json`. */
export interface PruneWorktree {
  name: string;
  path: string;
  /** Reclaimable/reclaimed size in bytes. */
  bytes: number;
  /** Whether the worktree is an orphaned/dangling entry. */
  orphaned: boolean;
  /** Human-readable warning, or `""`. */
  warning: string;
}

/** A worktree that prune declined to touch, with the reason. */
export interface PruneSkipped {
  name: string;
  path: string;
  /** Skip category (machine-readable bucket). */
  category: string;
  /** Short reason. */
  reason: string;
  /** Extended detail. */
  detail: string;
}

/** Full result of `awt prune --json`. */
export interface PruneResult {
  /** True when this was a dry run (no `--yes`). */
  dryRun: boolean;
  /** Worktrees that would be pruned (dry run) — the plan. */
  candidates: PruneWorktree[];
  /** Worktrees that were actually pruned. */
  pruned: PruneWorktree[];
  /** Worktrees skipped, with reasons. */
  skipped: PruneSkipped[];
  /** Total bytes reclaimable across candidates. */
  reclaimableBytes: number;
  /** Total bytes actually freed. */
  freedBytes: number;
  /** Number of pools swept — present only for `--all`. */
  poolCount?: number;
}

/** Classification a worktree can carry for `awt destroy`. */
export type DestroyClass =
  | "disposable"
  | "leased"
  | "in-use"
  | "dirty"
  | "unmerged"
  | "unverified";

/** A worktree planned for or actually destroyed — from `awt destroy --json`. */
export interface DestroyTarget {
  name: string;
  path: string;
  /** Every classification that applies to this target. */
  classes: DestroyClass[];
  /** Size in bytes. */
  bytes: number;
  /** Extended detail. */
  detail: string;
  /** Processes running inside the target. */
  processes: ProcessInfo[];
}

/** A worktree destroy declined to touch, with the flags it would need. */
export interface DestroySkip {
  name: string;
  path: string;
  classes: DestroyClass[];
  /** Flags that would have to be passed to include this target. */
  neededFlags: string[];
  /** True when skipped because a bulk op refused to touch a leased tree. */
  leasedBulk: boolean;
  detail: string;
}

/** Full result of `awt destroy --json`. */
export interface DestroyResult {
  /** True when this was a dry run (no `--yes`). */
  dryRun: boolean;
  /** Targets that would be destroyed (dry run) — the plan. */
  planned: DestroyTarget[];
  /** Targets that were actually destroyed. */
  destroyed: DestroyTarget[];
  /** Targets skipped, with the flags they'd need. */
  skipped: DestroySkip[];
  /** Total bytes across planned targets. */
  plannedBytes: number;
  /** Total bytes actually freed. */
  freedBytes: number;
}

/** Options accepted by the {@link AwtClient} constructor. */
export interface AwtClientOptions {
  /**
   * Explicit path to the `awt` binary. Takes priority over `AWT_BINARY`,
   * `PATH`, and the `go/awt` fallback.
   */
  binaryPath?: string;
  /** Working directory used when spawning the binary. */
  cwd?: string;
  /**
   * Extra environment variables merged into the child process environment.
   * `AWT_NO_UPDATE_CHECK=1` is always injected and cannot be overridden.
   */
  env?: Record<string, string>;
}

/** Options for {@link AwtClient.acquireLease}. */
export interface AcquireLeaseOptions {
  /** Lease-holder label (defaults to `AWT_LEASE_HOLDER` inside the binary). */
  holder?: string;
}

/** Options for {@link AwtClient.returnWorktree}. */
export interface ReturnOptions {
  /** Force the return even if the worktree is dirty or has live processes. */
  force?: boolean;
}

/** Options for {@link AwtClient.prune}. */
export interface PruneOptions {
  /** Actually perform the prune (otherwise it is a dry run). */
  yes?: boolean;
  /** Sweep every managed pool, not just the current one. */
  all?: boolean;
  /** Also prune orphaned/dangling worktree entries. */
  pruneOrphans?: boolean;
}

/** Options for {@link AwtClient.destroy}. */
export interface DestroyOptions {
  /** Treat `target` as a pool path and destroy all of its worktrees. */
  all?: boolean;
  /** Actually perform the destroy (otherwise it is a dry run). */
  yes?: boolean;
  /** Include worktrees whose work has not been landed/merged. */
  includeUnlanded?: boolean;
  /** Include worktrees with live processes. */
  includeInUse?: boolean;
  /** Include worktrees that are durably leased. */
  includeLeased?: boolean;
}

/** Context handed to the callback of {@link AwtClient.withWorktree}. */
export interface WorktreeContext {
  /** Absolute path to the leased worktree. */
  path: string;
}

/** Options for {@link AwtClient.withWorktree}. */
export interface WithWorktreeOptions {
  /** Lease-holder label passed through to {@link AwtClient.acquireLease}. */
  holder?: string;
  /**
   * When the callback throws, leave the worktree leased (skip the automatic
   * return) so its state can be inspected. Defaults to `false`.
   */
  keepOnError?: boolean;
}
