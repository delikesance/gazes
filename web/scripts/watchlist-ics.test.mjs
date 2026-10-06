// iCalendar text helpers of the "ma liste" export (RFC 5545 §3.1 folding, §3.3.11 escaping).
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "watchlist-ics.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/watchlist-ics.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { icsText, foldLine, mapLimited } = createRequire(import.meta.url)(outfile);

test("text escapes backslash before the other specials", () => {
  assert.equal(icsText("Fate\\Zero; Re:Zero, vol. 2\nfin"), "Fate\\\\Zero\\; Re:Zero\\, vol. 2\\nfin");
});

test("long lines fold at 75 octets without splitting a character", () => {
  const line = "SUMMARY:" + "épisode ".repeat(20);
  const folded = foldLine(line);
  const parts = folded.split("\r\n");
  assert.ok(parts.length > 1);
  for (const part of parts) assert.ok(Buffer.byteLength(part) <= 75, `${Buffer.byteLength(part)} octets: ${part}`);
  assert.equal(parts.map((p, i) => (i ? p.slice(1) : p)).join(""), line);
  assert.ok(parts.slice(1).every((p) => p.startsWith(" ")));
});

test("short lines are left alone", () => {
  assert.equal(foldLine("UID:1-2@gazes"), "UID:1-2@gazes");
});

test("franchise lookups are bounded in flight and keep the list order", async () => {
  let inFlight = 0, peak = 0;
  const ids = Array.from({ length: 40 }, (_, i) => i + 1);
  const out = await mapLimited(ids, 6, async (id) => {
    peak = Math.max(peak, ++inFlight);
    await new Promise((resolve) => setTimeout(resolve, id % 3));
    inFlight--;
    return id * 10;
  });
  assert.ok(peak <= 6, `peak ${peak}`);
  assert.deepEqual(out, ids.map((id) => id * 10));
});
