// Resolving an AniList list to franchise ids: dedup, and stop when lookups keep failing.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "anilist-import.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/anilist-import.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { resolveFranchises, MAX_CONSECUTIVE_FAILURES } = createRequire(import.meta.url)(outfile);

const entries = (...ids) => ids.map((id) => ({ id, title: `t${id}` }));

test("seasons of one franchise collapse into one id, order kept", async () => {
  const root = { 1: 100, 2: 100, 3: 300 };
  const out = await resolveFranchises(entries(1, 2, 3), async (id) => root[id]);
  assert.deepEqual(out, { franchiseIds: [100, 300], resolved: 3, skipped: 0, stopped: false });
});

test("a lone failure is skipped and the import goes on", async () => {
  const out = await resolveFranchises(entries(1, 2, 3), async (id) => { if (id === 2) throw new Error("x"); return id; });
  assert.deepEqual(out.franchiseIds, [1, 3]);
  assert.equal(out.skipped, 1);
  assert.equal(out.stopped, false);
});

test("consecutive failures stop the import without further lookups", async () => {
  let calls = 0;
  const out = await resolveFranchises(entries(1, 2, 3, 4, 5, 6, 7, 8), async () => { calls++; throw new Error("throttled"); });
  assert.equal(calls, MAX_CONSECUTIVE_FAILURES);
  assert.equal(out.stopped, true);
  assert.deepEqual(out.franchiseIds, []);
});

test("a success resets the failure streak", async () => {
  const fails = new Set([1, 2, 4, 5]);
  const out = await resolveFranchises(entries(1, 2, 3, 4, 5, 6), async (id) => { if (fails.has(id)) throw new Error("x"); return id; });
  assert.equal(out.stopped, false);
  assert.deepEqual(out.franchiseIds, [3, 6]);
});

test("progress reports each processed title and an aborted signal stops early", async () => {
  const seen = [];
  const controller = new AbortController();
  const out = await resolveFranchises(entries(1, 2, 3), async (id) => id, (n) => { seen.push(n); if (n === 2) controller.abort(); }, controller.signal);
  assert.deepEqual(seen, [1, 2]);
  assert.equal(out.stopped, true);
  assert.deepEqual(out.franchiseIds, [1, 2]);
});

const { parseMalExport } = createRequire(import.meta.url)(outfile);

test("a MAL export keeps watching, plan to watch and on hold titles only, each once", () => {
  const xml = `<?xml version="1.0"?><myanimelist>
    <anime><series_animedb_id>1</series_animedb_id><my_status>Watching</my_status></anime>
    <anime><series_animedb_id>2</series_animedb_id><my_status>Completed</my_status></anime>
    <anime><series_animedb_id><![CDATA[3]]></series_animedb_id><my_status><![CDATA[Plan to Watch]]></my_status></anime>
    <anime><series_animedb_id>4</series_animedb_id><my_status>Dropped</my_status></anime>
    <anime><series_animedb_id>5</series_animedb_id><my_status>On-Hold</my_status></anime>
    <anime><series_animedb_id>1</series_animedb_id><my_status>Watching</my_status></anime>
    <anime><series_animedb_id>6</series_animedb_id><my_status>6</my_status></anime>
    <anime><series_animedb_id>x</series_animedb_id><my_status>Watching</my_status></anime>
  </myanimelist>`;
  assert.deepEqual(parseMalExport(xml), [1, 3, 5, 6]);
  assert.deepEqual(parseMalExport("not xml"), []);
});
