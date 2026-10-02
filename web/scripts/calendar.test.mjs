// Unit tests for the release-calendar date helpers (src/lib/calendar.ts).
process.env.TZ = "Europe/Paris"; // a DST zone: the clocks change on 2026-10-25
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const tmp = await mkdtemp(path.join(tmpdir(), "calendar-"));
try {
  await build({ absWorkingDir: root, entryPoints: ["src/lib/calendar.ts"], outfile: path.join(tmp, "calendar.mjs"), bundle: true, format: "esm", platform: "node", logLevel: "silent" });
  const c = await import(path.join(tmp, "calendar.mjs"));

  // Weeks start on Monday, local time.
  const friday = new Date(2026, 9, 2, 15, 30);
  assert.equal(c.dayKey(c.startOfWeek(friday)), "2026-09-28");
  assert.equal(c.dayKey(c.startOfWeek(new Date(2026, 9, 4))), "2026-09-28", "Sunday belongs to the week that started on Monday");
  assert.equal(c.dayKey(c.startOfWeek(new Date(2026, 9, 5))), "2026-10-05");

  const week = c.weekRange(friday);
  assert.equal(c.daysIn(week).length, 7);
  assert.equal(c.dayKey(week.end), "2026-10-05", "the end is exclusive");

  // Month grid: October 2026 starts on a Thursday and ends on a Saturday: 5 whole weeks.
  const month = c.monthGridRange(new Date(2026, 9, 15));
  assert.equal(c.dayKey(month.start), "2026-09-28");
  assert.equal(c.dayKey(month.end), "2026-11-02");
  assert.equal(c.daysIn(month).length, 35);
  assert.equal(c.daysIn(c.monthGridRange(new Date(2027, 1, 10))).length, 28, "February 2027 (Monday to Sunday) fits four weeks");

  // Calendar arithmetic survives the DST change (a day is not always 24 hours).
  const beforeDst = new Date(2026, 9, 24);
  const after = c.addDays(beforeDst, 2);
  assert.equal(c.dayKey(after), "2026-10-26");
  assert.equal(after.getHours(), 0);
  assert.equal(c.daysIn(c.weekRange(new Date(2026, 9, 26))).length, 7);
  assert.equal(c.daysIn({ start: new Date(2026, 9, 19), end: new Date(2026, 9, 26) }).length, 7, "the 25-hour day is still one day");

  // API bounds are exclusive on both sides, in unix seconds.
  const { from, to } = c.apiBounds(week);
  assert.equal(from, week.start.getTime() / 1000 - 1);
  assert.equal(to, week.end.getTime() / 1000);

  // Entries group by local day and sort by time.
  const at = (y, m, d, h) => Math.floor(new Date(y, m, d, h).getTime() / 1000);
  const entry = (airing_at, media_id) => ({ airing_at, episode: 1, media_id, title: "T" + media_id });
  const entries = [entry(at(2026, 9, 2, 22), 1), entry(at(2026, 9, 2, 9), 2), entry(at(2026, 9, 3, 1), 3)];
  const groups = c.groupByDay(entries);
  assert.deepEqual(groups.get("2026-10-02").map((e) => e.media_id), [2, 1]);
  assert.equal(groups.get("2026-10-03").length, 1);

  // Airing states: past = aired, first upcoming of today = next, the rest = upcoming.
  const now = new Date(2026, 9, 2, 15, 0);
  const states = c.airingStates([...groups.get("2026-10-02"), ...groups.get("2026-10-03")], now);
  assert.equal(states.get(entries[1]), "aired");
  assert.equal(states.get(entries[0]), "next");
  assert.equal(states.get(entries[2]), "upcoming");
  console.log("calendar helpers: all checks passed");
} finally {
  await rm(tmp, { recursive: true, force: true });
}
