#!/usr/bin/env bun
/**
 * `awt-js` — a thin demo CLI over the SDK. It is deliberately NOT a replacement
 * for the real `awt`; it does two things:
 *
 *   1. Passthrough: any known `awt` subcommand is forwarded to the real binary
 *      with inherited stdio, so `awt-js status`, `awt-js get`, etc. behave
 *      exactly like `awt`.
 *   2. Value-add: `awt-js run [--holder <label>] -- <cmd...>` leases a worktree
 *      via {@link AwtClient.withWorktree}, runs `<cmd...>` inside it (with `cwd`
 *      and `AWT_DIR` pointed at the worktree), then returns the worktree.
 */

import { AwtClient } from "./client";
import { execInherit } from "./exec";
import { resolveBinary } from "./binary";

const USAGE = `awt-js — SDK demo CLI around the \`awt\` binary

Usage:
  awt-js run [--holder <label>] -- <cmd> [args...]   Lease a worktree, run cmd in it, return it
  awt-js <awt-subcommand> [args...]                  Passthrough to the real \`awt\` binary

Examples:
  awt-js run -- bun run agent.ts
  awt-js run --holder ci -- bash -lc "echo hi"
  awt-js status --json
`;

/** `awt-js run [--holder <label>] -- <cmd...>`. */
async function runInWorktree(rest: string[]): Promise<number> {
  const sep = rest.indexOf("--");
  if (sep === -1) {
    process.stderr.write("awt-js run: missing `--` before the command\n\n");
    process.stderr.write(USAGE);
    return 2;
  }

  const pre = rest.slice(0, sep);
  const [command, ...commandArgs] = rest.slice(sep + 1);
  if (!command) {
    process.stderr.write("awt-js run: no command given after `--`\n\n");
    process.stderr.write(USAGE);
    return 2;
  }

  let holder: string | undefined;
  for (let i = 0; i < pre.length; i++) {
    if (pre[i] === "--holder") {
      holder = pre[i + 1];
      i++;
    }
  }

  const client = new AwtClient();
  return client.withWorktree(
    ({ path }) =>
      execInherit(command, commandArgs, {
        cwd: path,
        env: { AWT_DIR: path },
      }),
    { holder },
  );
}

async function main(): Promise<number> {
  const argv = process.argv.slice(2);
  const [cmd] = argv;

  if (cmd === undefined || cmd === "--help" || cmd === "-h" || cmd === "help") {
    process.stdout.write(USAGE);
    return 0;
  }

  if (cmd === "run") {
    return runInWorktree(argv.slice(1));
  }

  // Passthrough to the real binary with inherited stdio.
  try {
    const binary = resolveBinary();
    return await execInherit(binary, argv);
  } catch (error) {
    process.stderr.write(
      `awt-js: ${error instanceof Error ? error.message : String(error)}\n`,
    );
    return 1;
  }
}

main().then(
  (code) => process.exit(code),
  (error: unknown) => {
    process.stderr.write(
      `awt-js: ${error instanceof Error ? error.message : String(error)}\n`,
    );
    process.exit(1);
  },
);
