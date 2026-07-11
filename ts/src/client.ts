/**
 * {@link AwtClient} — the programmatic surface of the SDK.
 *
 * Every method shells out to the resolved `awt` binary. The flagship helper is
 * {@link AwtClient.withWorktree}, which leases a worktree, runs your callback in
 * it, and reliably returns the worktree afterwards.
 */

import { resolveBinary } from "./binary";
import { execCapture, execInherit, type ExecOptions } from "./exec";
import type {
  AcquireLeaseOptions,
  AwtClientOptions,
  DestroyOptions,
  DestroyResult,
  PruneOptions,
  PruneResult,
  ReturnOptions,
  StatusResult,
  WithWorktreeOptions,
  WorktreeContext,
} from "./types";

/** Error thrown when a wrapped `awt` invocation exits non-zero. */
export class AwtError extends Error {
  /** The subcommand args that were passed to the binary. */
  readonly args: string[];
  /** The process exit code. */
  readonly code: number;
  /** Captured stderr, for diagnostics. */
  readonly stderr: string;

  constructor(args: string[], code: number, stderr: string) {
    const cmd = `awt ${args.join(" ")}`.trim();
    const detail = stderr.trim();
    super(
      `\`${cmd}\` exited with code ${code}` +
        (detail ? `:\n${detail}` : ""),
    );
    this.name = "AwtError";
    this.args = args;
    this.code = code;
    this.stderr = stderr;
  }
}

/**
 * A client bound to a single `awt` binary, working directory, and environment.
 *
 * @example
 * ```ts
 * const awt = new AwtClient();
 * await awt.withWorktree(async ({ path }) => {
 *   await runMyAgentIn(path);
 * });
 * ```
 */
export class AwtClient {
  /** Resolved path to the `awt` binary this client invokes. */
  readonly binaryPath: string;

  readonly #cwd?: string;
  readonly #env?: Record<string, string>;

  /**
   * @param options - Binary path override, working directory, and extra env.
   * @throws If the binary cannot be resolved (see {@link resolveBinary}).
   */
  constructor(options: AwtClientOptions = {}) {
    this.binaryPath = resolveBinary(options.binaryPath);
    this.#cwd = options.cwd;
    this.#env = options.env;
  }

  /** Common exec options (cwd + env) for this client. */
  #execOptions(extraEnv?: Record<string, string>): ExecOptions {
    const env =
      this.#env || extraEnv ? { ...this.#env, ...extraEnv } : undefined;
    return { cwd: this.#cwd, env };
  }

  /** Run the binary, capturing output; throw {@link AwtError} on non-zero. */
  async #run(args: string[]): Promise<string> {
    const { stdout, stderr, code } = await execCapture(
      this.binaryPath,
      args,
      this.#execOptions(),
    );
    if (code !== 0) throw new AwtError(args, code, stderr);
    return stdout;
  }

  /** `awt --version` — the binary's version string. */
  async version(): Promise<string> {
    return (await this.#run(["--version"])).trim();
  }

  /**
   * `awt get --lease [--lease-holder <holder>]` — acquire a worktree and mark
   * it durably leased.
   *
   * @returns The absolute path to the leased worktree.
   * @throws {@link AwtError} if no worktree could be acquired.
   */
  async acquireLease(options: AcquireLeaseOptions = {}): Promise<string> {
    const args = ["get", "--lease"];
    if (options.holder !== undefined) {
      args.push("--lease-holder", options.holder);
    }
    return (await this.#run(args)).trim();
  }

  /** `awt status --json` — the full pool status. */
  async status(): Promise<StatusResult> {
    const stdout = await this.#run(["status", "--json"]);
    return JSON.parse(stdout) as StatusResult;
  }

  /**
   * `awt enter <name> --print-path` — the absolute path of slot `name` without
   * opening a subshell.
   */
  async enterPath(name: string): Promise<string> {
    return (await this.#run(["enter", name, "--print-path"])).trim();
  }

  /**
   * `awt return [path] [--force]` — release a lease, reset the worktree, and
   * return it to the pool. With no `path`, the binary uses `$AWT_DIR`/cwd.
   */
  async returnWorktree(
    path?: string,
    options: ReturnOptions = {},
  ): Promise<void> {
    const args = ["return"];
    if (path !== undefined) args.push(path);
    if (options.force) args.push("--force");
    await this.#run(args);
  }

  /**
   * `awt prune [--yes] [--all] [--prune-orphans] --json` — reclaim reusable
   * worktrees. Without `yes` this is a dry run.
   */
  async prune(options: PruneOptions = {}): Promise<PruneResult> {
    const args = ["prune", "--json"];
    if (options.yes) args.push("--yes");
    if (options.all) args.push("--all");
    if (options.pruneOrphans) args.push("--prune-orphans");
    const stdout = await this.#run(args);
    return JSON.parse(stdout) as PruneResult;
  }

  /**
   * `awt destroy <target> [flags] --json` — permanently remove worktree(s).
   * Without `yes` this is a dry run.
   *
   * A single named target that is skipped for a missing flag makes the binary
   * exit non-zero while still printing a valid {@link DestroyResult}. That is a
   * *soft* outcome: the parsed result (with its `skipped` entries) is returned
   * rather than thrown. Only an unparseable payload throws.
   *
   * @throws {@link AwtError} if the binary produced no parseable JSON.
   */
  async destroy(
    target: string,
    options: DestroyOptions = {},
  ): Promise<DestroyResult> {
    const args = ["destroy", target, "--json"];
    if (options.all) args.push("--all");
    if (options.yes) args.push("--yes");
    if (options.includeUnlanded) args.push("--include-unlanded");
    if (options.includeInUse) args.push("--include-in-use");
    if (options.includeLeased) args.push("--include-leased");

    const { stdout, stderr, code } = await execCapture(
      this.binaryPath,
      args,
      this.#execOptions(),
    );

    try {
      return JSON.parse(stdout) as DestroyResult;
    } catch {
      // No parseable JSON — surface it as a hard error, preferring the real
      // exit code when the process failed.
      throw new AwtError(args, code === 0 ? 1 : code, stderr || stdout);
    }
  }

  /** `awt init` — write an `awt.toml` for the current repo. */
  async init(): Promise<void> {
    await this.#run(["init"]);
  }

  /**
   * Lease a worktree, run `fn` in it, and always return the worktree afterwards.
   *
   * This is the primary way a JS agent runner gets an isolated worktree: lease,
   * do work, release — reliably, even when `fn` throws. On success (and on
   * failure, unless `keepOnError` is set) the worktree is returned with
   * `--force`. When `keepOnError` is set and `fn` throws, the worktree is left
   * leased for inspection and the error is re-thrown.
   *
   * @param fn - Receives `{ path }` to the leased worktree; its result is
   *   returned.
   * @param options - `holder` label and `keepOnError` behavior.
   */
  async withWorktree<T>(
    fn: (ctx: WorktreeContext) => Promise<T>,
    options: WithWorktreeOptions = {},
  ): Promise<T> {
    const path = await this.acquireLease({ holder: options.holder });

    let result: T;
    try {
      result = await fn({ path });
    } catch (error) {
      if (!options.keepOnError) {
        // Best-effort cleanup; never mask the original failure with a
        // cleanup error.
        try {
          await this.returnWorktree(path, { force: true });
        } catch {
          /* swallow — the original error is what matters */
        }
      }
      throw error;
    }

    // fn succeeded: always return the worktree. A failure here is genuine
    // (the worktree could not be released) and is surfaced to the caller.
    await this.returnWorktree(path, { force: true });
    return result;
  }

  /**
   * Escape hatch for interactive passthrough: spawn the binary with the
   * parent's stdio inherited (e.g. `get`, `enter`, `status` table view).
   * Returns the child's exit code. Used by the demo CLI.
   */
  spawnInherit(args: string[], extraEnv?: Record<string, string>): Promise<number> {
    return execInherit(this.binaryPath, args, this.#execOptions(extraEnv));
  }
}
