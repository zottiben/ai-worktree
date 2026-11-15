# ai-worktree — project knowledge

A CLI dev tool (`awt`) for managing a pool of reusable, isolated **git worktrees**
so multiple AI coding agents can work in parallel — each in its own worktree —
without cloning, conflicts, or coordination overhead.

**Stack:** **Go core + TypeScript SDK**.
- `go/` — the engine and the `awt` binary (all worktree/pool logic; single static
  binary; Go 1.26, cobra CLI). This is the source of truth.
- `ts/` — a Bun/TypeScript SDK + demo CLI (`awt-js`) that shells out to the `awt`
  binary. Zero runtime dependencies; wraps the `--json` contract and exposes
  `withWorktree()` for JS/TS agent runners.

**Layout**
- `go/main.go` — entrypoint; `go/cmd/` — cobra commands (`get`, `enter`, `status`,
  `return`, `prune`, `destroy`, `init`, `update`).
- `go/internal/pool/` — pool manager (acquire/release/list/lease), state file
  (atomic write + flock + corrupt-state recovery), `prune.go`, `destroy.go`.
- `go/internal/git/` — git operations (shells out to the `git` binary).
- `go/internal/{config,hooks,process,shell,ui,updater}/` — config loading, lifecycle
  hooks, in-use detection + process termination, subshell spawning, prompts, self-update.
- `ts/src/` — `client.ts` (`AwtClient` + `withWorktree`), `binary.ts` (binary
  resolution), `exec.ts`, `types.ts`, `cli.ts`; `ts/test/` — bun tests + a fake-binary fixture.

**Commands** (from repo root)
- Build: `make build` (both) · `make -C go build` · `cd ts && bun run build`
- Lint: `make lint` (`gofmt -l` + `go vet` + `tsc --noEmit`)
- Test: `make test` (Go tests + `bun test`; Go CLI regression tests build `awt`, TS tests use a fake binary)
- Run (dev): `cd go && make build && ./awt`
- Requires **Go 1.26+** and **Bun 1.x**.

## Hard rules

### 1. Graduate conventions as code lands — keep this file true
Promote recurring decisions into concrete Hard rules here (each with its *why* and
how it's enforced), and keep **Commands** in sync with the real manifests
(`go/Makefile`, `Makefile`, `ts/package.json`). Never invent a command or a rule.
Run `/lint` on this file periodically so it doesn't rot.

### 2. Worktree, branch, and force git operations are destructive — guard them
This tool's whole job is creating and tearing down git worktrees. Those operations
(`worktree remove --force`, hard reset, clean) can silently lose uncommitted or
unpushed work. The implemented, tested guarantees:

- **Explicit targets only.** `destroy <name|path>` and `return <name|path>` act
  on one worktree through one shared resolver (`go/cmd/target.go`); bare names
  resolve only within the current repository's pool, using the owning
  checkout's config even inside a linked worktree. Existing relative paths and
  explicit paths never fall back to name lookup, avoiding accidental deletion of
  a different target. Paths match the recorded worktree by file identity, so
  symlink aliases resolve, but never by containment; only a bare `awt return`
  takes the worktree that contains the current directory (covered in
  `go/cmd/destroy_test.go` and `go/cmd/return_test.go`). `destroy <pool> --all`
  acts on that pool only. There is NO cross-pool/global destroy; `--all` without
  a pool path is an error. Never act on a sibling or the primary checkout by
  inference.
- **Dry-run by default.** `prune` and `destroy` preview and remove nothing unless
  `--yes` is passed.
- **Per-risk opt-in.** A bare `destroy --yes` removes only the disposable set
  (merged, clean, idle, unleased). Each risky class is its own flag:
  `--include-unlanded` (dirty/unmerged/unverified), `--include-in-use` (running
  process), `--include-leased` (leased — single name or path only, NEVER via `--all`).
- **Leases and dirty state are protected.** Leased worktrees are never handed out
  by `get`, never removed by normal prune, and never bulk-destroyed. Dirty detection includes
  untracked files even when git config hides them.
- **A machine restart preserves in-use worktrees.** An interactive `awt get`
  reservation is process-scoped, but the boot time is recorded alongside it so a
  reboot is told apart from a crash. A crash (same boot session, owner gone)
  reclaims the worktree; a reboot converts the reservation into a durable lease
  so the worktree is never reset or handed out and its committed-but-unpushed
  work survives — resume with `awt enter` or release with `awt return`.
- **Git registrations reserve their paths.** `status` and `get` reconcile the
  current pool with Git's registered worktrees, including named slots. Unknown
  entries are recovered as leased. Missing checkouts remain visible and keep
  their leases, but do not consume `max_trees` capacity. Acquisition skips their
  paths without pruning or force-overwriting registrations, preserving detached
  commits for recovery (covered in `go/cmd/worktree_registry_test.go` and
  `go/internal/pool/reconcile_test.go`).
- **Missing cleanup is metadata-only and opt-in.** `prune --missing` previews
  only the current repository's pool; `--yes` applies it. It clears automatic
  recovery holds only on absent checkout paths, never explicit leases, active
  owners or Git locks. Registered HEAD commits are pinned under
  `refs/awt/recovered/` before targeted, non-force Git removal. Existing paths
  (including broken symlinks) and other pools are untouched, and hooks do not
  run. Tests cover this in `go/cmd/prune_missing_test.go`,
  `go/internal/pool/prune_missing_test.go` and `go/internal/git/missing_test.go`.
- **Corrupt state fails safe.** A truncated `awt-state.json` is recovered by
  scanning the pool dir and marking every rebuilt entry leased until a human
  verifies it — never silently reused or deleted.

These are backed by tests in `go/internal/pool/` (`destroy_test.go`,
`prune_test.go`, `state_test.go`, `pool_test.go`). Any change to removal/lease
semantics MUST keep those green and add a case for the new behavior.

### 3. One repository, one pool
Every command that reads or changes a pool resolves the repository through the
owning checkout (`git.FindMainRepoRootFrom`), never a linked worktree's own root.
Without a remote the pool name hashes the repository path, and a relative `root`
in `awt.toml` resolves from the repository root, so a linked worktree's root names
a different pool and awt loses track of worktrees it manages. Covered by
`go/cmd/enter_test.go`, the `get-linked` case in `go/cmd/worktree_registry_test.go`
and `TestDestroyByName` in `go/cmd/destroy_test.go`.
