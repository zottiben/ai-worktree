/**
 * Resolution of the `awt` Go binary.
 *
 * The SDK never bundles the binary; it locates an installed one. Resolution
 * priority (highest first):
 *   1. an explicit `override` (the client's `binaryPath` option)
 *   2. the `AWT_BINARY` environment variable
 *   3. `awt` on `PATH`
 *   4. the sibling `go/awt` build in this monorepo (`../go/awt` from `ts/`)
 */

import { accessSync, constants, statSync } from "node:fs";
import { delimiter, join, resolve } from "node:path";

/** True when `p` exists and is a regular file. */
function isFile(p: string): boolean {
  try {
    return statSync(p).isFile();
  } catch {
    return false;
  }
}

/** True when `p` is a regular file with the executable bit set. */
function isExecutableFile(p: string): boolean {
  if (!isFile(p)) return false;
  try {
    accessSync(p, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

/** Find an executable named `name` on `PATH`, or `undefined`. */
function findOnPath(name: string): string | undefined {
  const rawPath = process.env.PATH;
  if (!rawPath) return undefined;
  for (const dir of rawPath.split(delimiter)) {
    if (!dir) continue;
    const candidate = join(dir, name);
    if (isExecutableFile(candidate)) return candidate;
  }
  return undefined;
}

/**
 * Absolute path to the sibling `go/awt` build, or `undefined` if it does not
 * exist. `binary.ts` lives at `ts/src/binary.ts`, so `../../go/awt` resolves to
 * `<repoRoot>/go/awt`.
 */
export function goFallbackPath(): string | undefined {
  const candidate = resolve(import.meta.dir, "../../go/awt");
  return isFile(candidate) ? candidate : undefined;
}

/** Options for {@link resolveBinary}. */
export interface ResolveBinaryOptions {
  /**
   * Override for the sibling-build fallback (step 4). Defaults to
   * {@link goFallbackPath}. Primarily a testing seam so the exhausted-resolution
   * path can be exercised regardless of whether `go/awt` happens to be built.
   */
  fallback?: () => string | undefined;
}

/**
 * Resolve the `awt` binary path following the documented priority order.
 *
 * @param override - Explicit binary path (the client's `binaryPath` option).
 * @param opts - Optional resolution seams (see {@link ResolveBinaryOptions}).
 * @returns The resolved absolute (or `PATH`-relative) binary path.
 * @throws If no binary can be found, with guidance on how to provide one.
 */
export function resolveBinary(
  override?: string,
  opts: ResolveBinaryOptions = {},
): string {
  if (override) {
    if (isFile(override)) return override;
    throw new Error(
      `awt binary not found at the provided path: ${override}`,
    );
  }

  const fromEnv = process.env.AWT_BINARY;
  if (fromEnv) {
    if (isFile(fromEnv)) return fromEnv;
    throw new Error(
      `AWT_BINARY is set to a path that is not a file: ${fromEnv}`,
    );
  }

  const onPath = findOnPath("awt");
  if (onPath) return onPath;

  const fallback = (opts.fallback ?? goFallbackPath)();
  if (fallback) return fallback;

  throw new Error(
    "Could not locate the `awt` binary. Set AWT_BINARY to its absolute path, " +
      "put `awt` on your PATH, pass `binaryPath` to AwtClient, or build the " +
      "sibling `go/awt` binary.",
  );
}
