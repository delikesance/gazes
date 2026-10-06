// Manual watched marks: they override what the resume point implies, and never store a redundant override.
import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "watched.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/watched.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });

const store = new Map();
globalThis.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
  key: (i) => [...store.keys()][i] ?? null,
};
Object.defineProperty(globalThis.localStorage, "length", { get: () => store.size });
globalThis.window = { dispatchEvent() {}, addEventListener() {}, removeEventListener() {} };
globalThis.Event = class {};
const { parseMarks, isWatched, toggleWatched, clearWatchedMarks } = createRequire(import.meta.url)(outfile);
const read = (season) => parseMarks(localStorage.getItem(`gazes-watched:${season}`));

beforeEach(() => store.clear());

test("episodes before the resume point count as watched, the others do not", () => {
  const marks = parseMarks(null);
  assert.equal(isWatched(marks, 2, 5), true);
  assert.equal(isWatched(marks, 5, 5), false);
  assert.equal(isWatched(marks, 6, 5), false);
  assert.equal(isWatched(marks, 1, undefined), false);
});

test("marking an episode past the resume point stores a seen override, and toggling again drops it", () => {
  toggleWatched(7, 9, 5);
  assert.deepEqual(read(7), { seen: [9], unseen: [] });
  toggleWatched(7, 9, 5);
  assert.equal(localStorage.getItem("gazes-watched:7"), null);
});

test("clearing an episode the resume point implies stores an unseen override, and toggling again drops it", () => {
  toggleWatched(7, 2, 5);
  assert.deepEqual(read(7), { seen: [], unseen: [2] });
  assert.equal(isWatched(read(7), 2, 5), false);
  toggleWatched(7, 2, 5);
  assert.equal(localStorage.getItem("gazes-watched:7"), null);
});

test("without a resume point a toggle marks, then clears", () => {
  toggleWatched(3, 4, undefined);
  assert.equal(isWatched(read(3), 4, undefined), true);
  toggleWatched(3, 4, undefined);
  assert.equal(isWatched(read(3), 4, undefined), false);
});

test("garbage in storage is ignored and clearing removes every season", () => {
  localStorage.setItem("gazes-watched:1", "{nope");
  assert.deepEqual(read(1), { seen: [], unseen: [] });
  toggleWatched(1, 1, undefined);
  toggleWatched(2, 1, undefined);
  localStorage.setItem("other", "keep");
  clearWatchedMarks();
  assert.equal(localStorage.getItem("gazes-watched:1"), null);
  assert.equal(localStorage.getItem("gazes-watched:2"), null);
  assert.equal(localStorage.getItem("other"), "keep");
});
