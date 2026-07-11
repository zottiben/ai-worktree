import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { join } from "node:path";
import { resolveBinary } from "../src/binary";

const FAKE = join(import.meta.dir, "fixtures", "fake-awt");

describe("resolveBinary", () => {
  let savedEnv: NodeJS.ProcessEnv;

  beforeEach(() => {
    savedEnv = { ...process.env };
  });
  afterEach(() => {
    process.env = savedEnv;
  });

  test("prefers an explicit override", () => {
    process.env.AWT_BINARY = "/does/not/exist";
    expect(resolveBinary(FAKE)).toBe(FAKE);
  });

  test("throws when the override path is not a file", () => {
    expect(() => resolveBinary("/no/such/awt")).toThrow(/not found/);
  });

  test("falls back to AWT_BINARY when no override is given", () => {
    process.env.AWT_BINARY = FAKE;
    expect(resolveBinary()).toBe(FAKE);
  });

  test("throws when AWT_BINARY points at a non-file", () => {
    process.env.AWT_BINARY = "/no/such/awt";
    expect(() => resolveBinary()).toThrow(/AWT_BINARY/);
  });

  test("throws a helpful error when nothing resolves", () => {
    delete process.env.AWT_BINARY;
    process.env.PATH = ""; // no `awt` discoverable on PATH
    // Force the sibling-build fallback to miss too, so this is hermetic whether
    // or not go/awt happens to be built in this checkout.
    expect(() => resolveBinary(undefined, { fallback: () => undefined })).toThrow(
      /Could not locate the `awt` binary/,
    );
  });
});
