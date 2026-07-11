import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { AwtClient } from "../src/client";

const FAKE = join(import.meta.dir, "fixtures", "fake-awt");

let logDir: string;
let logPath: string;

/** A client whose invocations are logged to `logPath` by the fake binary. */
function makeClient(): AwtClient {
  return new AwtClient({ binaryPath: FAKE, env: { FAKE_AWT_LOG: logPath } });
}

/** The argv lines the fake recorded, one per invocation. */
function readLog(): string[] {
  try {
    return readFileSync(logPath, "utf8").trim().split("\n").filter(Boolean);
  } catch {
    return [];
  }
}

beforeEach(() => {
  logDir = mkdtempSync(join(tmpdir(), "awt-sdk-test-"));
  logPath = join(logDir, "invocations.log");
});
afterEach(() => {
  rmSync(logDir, { recursive: true, force: true });
});

describe("AwtClient basic commands", () => {
  test("version() returns the trimmed version string", async () => {
    expect(await makeClient().version()).toBe("awt 0.0.0-fake");
  });

  test("acquireLease() returns only the stdout path", async () => {
    expect(await makeClient().acquireLease()).toBe("/tmp/awt-fake-pool/1");
    expect(readLog()[0]).toBe("get --lease");
  });

  test("acquireLease({ holder }) forwards --lease-holder", async () => {
    await makeClient().acquireLease({ holder: "agent-x" });
    expect(readLog()[0]).toBe("get --lease --lease-holder agent-x");
  });

  test("status() parses the StatusJSON payload", async () => {
    const result = await makeClient().status();
    expect(result.poolDir).toBe("/tmp/awt-fake-pool");
    expect(result.worktrees).toHaveLength(2);
    expect(result.worktrees[0]).toMatchObject({
      name: "1",
      status: "leased",
      leaseHolder: "agent-a",
    });
    expect(result.worktrees[1]?.processes[0]).toEqual({
      pid: 4242,
      name: "bun",
    });
  });

  test("enterPath() prints the slot path", async () => {
    expect(await makeClient().enterPath("2")).toBe("/tmp/awt-fake-pool/2");
    expect(readLog()[0]).toBe("enter 2 --print-path");
  });

  test("returnWorktree() forwards path and --force", async () => {
    await makeClient().returnWorktree("/tmp/awt-fake-pool/1", { force: true });
    expect(readLog()[0]).toBe("return /tmp/awt-fake-pool/1 --force");
  });

  test("prune() parses the PruneJSON payload", async () => {
    const result = await makeClient().prune({ yes: true });
    expect(result.pruned).toHaveLength(1);
    expect(result.pruned[0]?.bytes).toBe(1024);
    expect(result.freedBytes).toBe(1024);
    expect(readLog()[0]).toBe("prune --json --yes");
  });
});

describe("AwtClient.destroy soft-skip handling", () => {
  test("returns the parsed result even when the binary exits non-zero", async () => {
    // The fake exits 1 for a single-target skip but prints valid DestroyJSON.
    const result = await makeClient().destroy("/tmp/awt-fake-pool/1");
    expect(result.skipped).toHaveLength(1);
    expect(result.skipped[0]?.neededFlags).toEqual(["--include-leased"]);
    expect(result.destroyed).toHaveLength(0);
  });

  test("destroys when the needed flag is provided (exit 0)", async () => {
    const result = await makeClient().destroy("/tmp/awt-fake-pool/1", {
      includeLeased: true,
      yes: true,
    });
    expect(result.destroyed).toHaveLength(1);
    expect(result.freedBytes).toBe(2048);
  });
});

describe("AwtClient.withWorktree", () => {
  test("returns the callback result and always returns the worktree", async () => {
    const seen: string[] = [];
    const value = await makeClient().withWorktree(async ({ path }) => {
      seen.push(path);
      return 42;
    });
    expect(value).toBe(42);
    expect(seen).toEqual(["/tmp/awt-fake-pool/1"]);

    const log = readLog();
    expect(log[0]).toBe("get --lease");
    expect(log).toContain("return /tmp/awt-fake-pool/1 --force");
  });

  test("returns the worktree even when the callback throws", async () => {
    const boom = new Error("agent blew up");
    await expect(
      makeClient().withWorktree(async () => {
        throw boom;
      }),
    ).rejects.toBe(boom);

    // Despite the failure, the worktree was returned.
    expect(readLog()).toContain("return /tmp/awt-fake-pool/1 --force");
  });

  test("keepOnError leaves the worktree leased when the callback throws", async () => {
    const boom = new Error("keep me");
    await expect(
      makeClient().withWorktree(
        async () => {
          throw boom;
        },
        { keepOnError: true },
      ),
    ).rejects.toBe(boom);

    const log = readLog();
    expect(log[0]).toBe("get --lease");
    // No return was issued — the worktree is left leased for inspection.
    expect(log.some((line) => line.startsWith("return"))).toBe(false);
  });

  test("forwards the holder label to the lease", async () => {
    await makeClient().withWorktree(async () => undefined, { holder: "ci" });
    expect(readLog()[0]).toBe("get --lease --lease-holder ci");
  });
});
