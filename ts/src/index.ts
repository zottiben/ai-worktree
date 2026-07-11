/**
 * `@ai-worktree/sdk` — a Bun/TypeScript SDK and CLI around the `awt` Go binary.
 *
 * Import the {@link AwtClient} class for full control, or the bound convenience
 * functions for the common single-client case.
 *
 * @example
 * ```ts
 * import { withWorktree } from "@ai-worktree/sdk";
 *
 * await withWorktree(async ({ path }) => {
 *   // run an agent inside `path`; the worktree is returned automatically.
 * });
 * ```
 */

export { AwtClient, AwtError } from "./client";
export { resolveBinary, goFallbackPath } from "./binary";
export {
  execCapture,
  execInherit,
  type ExecOptions,
  type ExecResult,
} from "./exec";
export type * from "./types";

import { AwtClient } from "./client";
import type {
  AcquireLeaseOptions,
  StatusResult,
  WithWorktreeOptions,
  WorktreeContext,
} from "./types";

let defaultClient: AwtClient | undefined;

/**
 * The lazily-created default client (binary resolved on first use, not at
 * import time — so importing this module never throws when no binary exists).
 */
export function getDefaultClient(): AwtClient {
  return (defaultClient ??= new AwtClient());
}

/** {@link AwtClient.withWorktree} bound to the default client. */
export function withWorktree<T>(
  fn: (ctx: WorktreeContext) => Promise<T>,
  options?: WithWorktreeOptions,
): Promise<T> {
  return getDefaultClient().withWorktree(fn, options);
}

/** {@link AwtClient.status} bound to the default client. */
export function status(): Promise<StatusResult> {
  return getDefaultClient().status();
}

/** {@link AwtClient.acquireLease} bound to the default client. */
export function acquireLease(options?: AcquireLeaseOptions): Promise<string> {
  return getDefaultClient().acquireLease(options);
}

/** {@link AwtClient.version} bound to the default client. */
export function version(): Promise<string> {
  return getDefaultClient().version();
}
