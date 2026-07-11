/**
 * Thin, typed wrapper around spawning the `awt` binary.
 *
 * Two modes:
 *   - {@link execCapture} buffers stdout/stderr and resolves with the exit code
 *     (never rejects on a non-zero exit — callers decide what a code means).
 *   - {@link execInherit} inherits the parent's stdio, for interactive
 *     passthrough (subshells, live command output).
 *
 * Both always inject `AWT_NO_UPDATE_CHECK=1` so wrapped calls stay quiet and
 * deterministic; it cannot be overridden by `opts.env`.
 */

import { spawn } from "node:child_process";

/** Result of a captured (buffered) binary invocation. */
export interface ExecResult {
  /** Everything the process wrote to stdout. */
  stdout: string;
  /** Everything the process wrote to stderr. */
  stderr: string;
  /** Exit code (1 is substituted when the process was killed by a signal). */
  code: number;
}

/** Options shared by both exec modes. */
export interface ExecOptions {
  /** Working directory for the child process. */
  cwd?: string;
  /** Extra environment variables (merged over `process.env`). */
  env?: Record<string, string>;
}

/**
 * Build the child environment: inherit the parent env, layer `opts.env` on top,
 * then force `AWT_NO_UPDATE_CHECK=1` last so it always wins.
 */
function buildEnv(extra?: Record<string, string>): NodeJS.ProcessEnv {
  return { ...process.env, ...extra, AWT_NO_UPDATE_CHECK: "1" };
}

/**
 * Spawn `binary args...`, buffering stdout and stderr.
 *
 * Resolves with `{ stdout, stderr, code }` for any exit code. Rejects only when
 * the process cannot be spawned at all (e.g. the binary is missing).
 */
export function execCapture(
  binary: string,
  args: string[],
  opts: ExecOptions = {},
): Promise<ExecResult> {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(binary, args, {
      cwd: opts.cwd,
      env: buildEnv(opts.env),
      stdio: ["ignore", "pipe", "pipe"],
    });

    let stdout = "";
    let stderr = "";
    child.stdout?.setEncoding("utf8");
    child.stderr?.setEncoding("utf8");
    child.stdout?.on("data", (chunk: string) => {
      stdout += chunk;
    });
    child.stderr?.on("data", (chunk: string) => {
      stderr += chunk;
    });

    child.on("error", reject);
    child.on("close", (code) => {
      resolvePromise({ stdout, stderr, code: code ?? 1 });
    });
  });
}

/**
 * Spawn `binary args...` with the parent's stdio inherited (interactive).
 *
 * Resolves with the exit code. Rejects only on a spawn error.
 */
export function execInherit(
  binary: string,
  args: string[],
  opts: ExecOptions = {},
): Promise<number> {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(binary, args, {
      cwd: opts.cwd,
      env: buildEnv(opts.env),
      stdio: "inherit",
    });
    child.on("error", reject);
    child.on("close", (code) => {
      resolvePromise(code ?? 1);
    });
  });
}
