// "Ma liste" sync after sign-in: what this device uploads and what it keeps, given the server list.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "watchlist-sync.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/watchlist-sync.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { planWatchlistSync, chunks } = createRequire(import.meta.url)(outfile);

test("items saved on this device and never synced are uploaded", () => {
  const plan = planWatchlistSync([3, 1, 2], [1, 2], [1, 2]);
  assert.deepEqual(plan.upload, [3]);
  assert.deepEqual(plan.keep, [3, 1, 2]);
});

test("an item removed on another device does not come back", () => {
  // 7 was on the server at the last sync and is gone now: it was removed elsewhere.
  const plan = planWatchlistSync([7, 1], [1], [7, 1]);
  assert.deepEqual(plan.upload, []);
  assert.deepEqual(plan.keep, [1]);
});

test("first sync on a device uploads everything missing", () => {
  assert.deepEqual(planWatchlistSync([5, 6], [6], []).upload, [5]);
});

test("uploads are split under the server's 500 ids per request", () => {
  const ids = Array.from({ length: 1201 }, (_, i) => i + 1);
  const parts = chunks(ids, 500);
  assert.deepEqual(parts.map((p) => p.length), [500, 500, 201]);
  assert.deepEqual(parts.flat(), ids);
});
