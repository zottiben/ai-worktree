# @ai-worktree/sdk

A tiny **Bun/TypeScript** SDK and demo CLI around the [`awt`](../go) Go binary,
which manages a per-repo pool of reusable **git worktrees** so multiple AI
coding agents can work in parallel — each in its own isolated worktree/branch.

The SDK's job is to make the durable-**lease** lifecycle (acquire → work →
return) ergonomic and reliable from a JS/TS agent runner. It has **zero runtime
dependencies** — only Bun/Node built-ins.

## Install

```sh
bun add @ai-worktree/sdk
```

The SDK shells out to the `awt` binary; it does not bundle it. See
[Locating the `awt` binary](#locating-the-awt-binary).

## The `withWorktree` flow

`withWorktree` leases a worktree, runs your callback inside it, and **always**
returns the worktree afterwards — even if the callback throws — so leases never
leak.

```ts
import { withWorktree } from "@ai-worktree/sdk";

const summary = await withWorktree(async ({ path }) => {
  // `path` is an isolated git worktree. Point your agent at it.
  await runMyAgentIn(path);
  return "done";
});
// worktree already returned to the pool here (with --force).
```

Need control over the binary, working directory, or environment? Use the class:

```ts
import { AwtClient } from "@ai-worktree/sdk";

const awt = new AwtClient({ binaryPath: "/opt/bin/awt" });

await awt.withWorktree(
  async ({ path }) => runMyAgentIn(path),
  { holder: "ci", keepOnError: true }, // leave leased if the agent throws
);
```

### Other methods

```ts
const awt = new AwtClient();
await awt.version();                       // "awt x.y.z"
await awt.acquireLease({ holder: "ci" });  // -> leased worktree path
await awt.status();                        // parsed StatusResult
await awt.enterPath("1");                  // slot path, no subshell
await awt.returnWorktree(path, { force: true });
await awt.prune({ yes: true });            // parsed PruneResult
await awt.destroy(path, { includeLeased: true, yes: true }); // DestroyResult
await awt.init();
```

`destroy` is intentionally forgiving: when a single named target is skipped for
a missing flag the binary exits non-zero but still prints a valid result — the
SDK returns that parsed `DestroyResult` (check `.skipped`) instead of throwing.
It only throws when the output is unparseable.

## Locating the `awt` binary

`resolveBinary` (used by every client) tries, in order:

1. an explicit `binaryPath` passed to `new AwtClient({ binaryPath })`;
2. the `AWT_BINARY` environment variable (absolute path);
3. `awt` on your `PATH`;
4. the sibling `go/awt` build in this monorepo (`../go/awt` from `ts/`).

If none resolve, the client constructor throws with guidance.

Every wrapped invocation is spawned with `AWT_NO_UPDATE_CHECK=1` so calls stay
quiet and deterministic.

## Demo CLI: `awt-js`

`awt-js` is a thin wrapper — **not** a replacement for `awt`. It forwards known
subcommands to the real binary, and adds one value-add command that showcases
the SDK:

```sh
# Lease a worktree, run a command inside it (cwd + $AWT_DIR set), then return it:
awt-js run -- bun run agent.ts
awt-js run --holder ci -- bash -lc "echo working in $AWT_DIR"

# Everything else is passed straight through to `awt`:
awt-js status --json
```

## Development

```sh
bun install
bun run typecheck   # tsc --noEmit
bun test            # bun test (uses a fake binary fixture; no real awt needed)
bun run build       # ESM library build to dist/
```
