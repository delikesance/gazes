// Notes sync: parsing of stored notes and the newest-wins merge with the server copy.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "notes.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/notes.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { parseNotes, mergeNotes, MAX_NOTE_LENGTH } = createRequire(import.meta.url)(outfile);

test("garbage and out-of-range values are cleaned up when parsing", () => {
  assert.deepEqual(parseNotes("{nope"), {});
  assert.deepEqual(parseNotes(null), {});
  const parsed = parseNotes(JSON.stringify({ 5: { rating: 99, note: "x".repeat(MAX_NOTE_LENGTH + 20), updatedAt: 10 }, abc: { rating: 1, note: "", updatedAt: 1 }, 6: { rating: 1.5, note: "", updatedAt: 1 } }));
  assert.deepEqual(Object.keys(parsed), ["5"]);
  assert.equal(parsed[5].rating, 10);
  assert.equal(parsed[5].note.length, MAX_NOTE_LENGTH);
});

test("merge takes the newer side per anime and pushes what the server lacks or has older", () => {
  const local = { 1: { rating: 5, note: "mine", updatedAt: 100 }, 2: { rating: 3, note: "old local", updatedAt: 50 }, 3: { rating: 7, note: "only local", updatedAt: 10 } };
  const remote = [
    { anime_id: 1, rating: 9, note: "older remote", updated_at: 90 },
    { anime_id: 2, rating: 8, note: "newer remote", updated_at: 60 },
    { anime_id: 4, rating: 2, note: "only remote", updated_at: 5 },
  ];
  const { merged, push } = mergeNotes(local, remote);
  assert.equal(merged[1].note, "mine");
  assert.equal(merged[2].note, "newer remote");
  assert.equal(merged[3].note, "only local");
  assert.equal(merged[4].note, "only remote");
  assert.deepEqual(push.map((n) => n.anime_id).sort(), [1, 3]);
});

test("a local clear newer than the server copy is pushed as a tombstone", () => {
  const { merged, push } = mergeNotes({ 1: { rating: 0, note: "", updatedAt: 200 } }, [{ anime_id: 1, rating: 8, note: "x", updated_at: 100 }]);
  assert.equal(merged[1].rating, 0);
  assert.deepEqual(push, [{ anime_id: 1, rating: 0, note: "", updated_at: 200 }]);
});
