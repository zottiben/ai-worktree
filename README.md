<h1 align="center">ai-worktree (<code>awt</code>)</h1>

<p align="center"><em>A pool of reusable git worktrees, so multiple AI coding agents can work in parallel — no cloning, no conflicts.</em></p>

`awt` hands each agent its own isolated git worktree instantly, then reclaims it
for the next one — with dependencies and build cache intact. No daemon.

```
ai-worktree/
├── go/     # the `awt` binary
└── ts/     # a TypeScript SDK for driving awt from JS/TS agent runners
```

## Install

**macOS / Linux:**

```sh
curl -fsSL https://zottiben.github.io/ai-worktree/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://zottiben.github.io/ai-worktree/install.ps1 | iex
```

<details>
<summary>Build from source (requires Go 1.26+)</summary>

```sh
cd go && make build          # builds ./go/awt
cp go/awt ~/.local/bin/       # or anywhere on your PATH
```
</details>

**SDK** (optional, for driving `awt` from JS/TS) — requires Bun 1.x:

```sh
cd ts && bun install
```

## Setup

None required — `awt` works out of the box. Optionally drop an `awt.toml` in your
repo to tune the pool:

```sh
cd myproject
awt init                      # writes awt.toml (max_trees, root)
```

## Usage

### CLI

```sh
cd myproject
awt                           # get a worktree and open a subshell in it
# ... run your agent, make changes ...
exit                          # worktree is cleaned and returned to the pool
```

| Command | What it does |
| --- | --- |
| `awt` / `awt get` | Acquire a worktree and open a subshell (returns it on exit) |
| `awt get --lease` | Reserve a worktree without a subshell; prints its path to stdout |
| `awt enter <n>` | Open a subshell in worktree `n`, even if it's in use |
| `awt status` | Show the pool (add `--json` for machine-readable output) |
| `awt return [path]` | Release, clean, and return a worktree to the pool |
| `awt prune` | Remove stale idle worktrees (dry run; `--yes` to delete) |
| `awt prune --missing` | Clean missing checkout registrations in this pool (dry run; `--yes` to apply) |
| `awt destroy <name-or-path>` | Remove a worktree by name or path (dry run; `--yes` to delete) |
| `awt init` | Create an `awt.toml` |
| `awt update` | Update to the latest release |

`prune` and `destroy` are dry-run by default and only remove worktrees that are
idle, clean, and merged. Removing anything riskier (dirty, unmerged, in-use, or
leased) requires an explicit `--include-*` flag — run either command to see which.

`awt destroy 5` selects worktree `5` from the current repository's pool, including
when run inside a linked worktree. An existing relative path is treated as a
path, not a name; use `./path` to force path lookup. Bulk destruction still
requires a pool path: `awt destroy <pool-path> --all`.

`awt status` and `awt get` reconcile pool state with Git's registered worktrees
inside that pool. A missing checkout is shown as `missing`; its path is not reused
and it does not count towards `max_trees`. Git registrations are left intact to
preserve recovery information. Worktrees absent from pool state are recovered as
leased, including named slots. Verify them before using `awt return` to reset and
release them. Existing leases stay protected if a missing checkout returns.

For directories already deleted from disk:

```sh
awt prune --missing          # preview missing registrations and stale state entries
awt prune --missing --yes    # apply the cleanup
```

This mode only affects the current repository's pool. It saves each registered
HEAD under `refs/awt/recovered/<commit>` before removing the Git registration and
pool entry. Those commits remain available after Git garbage collection; inspect
one with `git show refs/awt/recovered/<commit>`, or create a branch from that ref.
Deleted working files are not restored. Automatic recovery holds can be cleared,
but explicit leases, Git locks, running owners, and paths that still exist are
protected. No checkout files are deleted and no lifecycle hooks run. This mode
cannot be combined with `--all`, `--global`, or `--prune-orphans`.

### SDK

Lease a worktree, run your agent in it, and always return it afterwards:

```ts
import { AwtClient } from "@ai-worktree/sdk";

const awt = new AwtClient();

const result = await awt.withWorktree(async ({ path }) => {
  return await runMyAgentIn(path);
});

// Or drive the pool directly:
const { worktrees } = await awt.status();
await awt.prune({ yes: true });
```

The SDK finds `awt` on your `PATH` (or the sibling `go/awt` build). See
[`ts/README.md`](ts/README.md) for the full API.

## Configuration

`awt.toml` in the repo root, or `~/.config/awt/config.toml` for user-level
settings and `[hooks]`:

```toml
max_trees = 16
# root = "$HOME/worktrees"

# [hooks]  (user-level config only)
# post_create = ["./scripts/setup.sh"]
# pre_destroy = ["./scripts/teardown.sh"]
```

## Development

```sh
make build      # build the binary + the SDK
make test       # go tests + ts typecheck/tests
make lint       # gofmt + go vet + tsc --noEmit
```
