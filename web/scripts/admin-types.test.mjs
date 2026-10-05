// Checks web/src/lib/admin/types.ts against REAL admin API responses
// (web/src/lib/admin/real/*.json, dumped from the Go handlers).
//   1. tsc: every JSON must match its interface exactly (see real/check.ts).
//   2. runtime: string-literal unions that JSON imports cannot express.
// Run: cd web && node scripts/admin-types.test.mjs
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const real = (name) =>
  JSON.parse(readFileSync(new URL(`../src/lib/admin/real/${name}.json`, import.meta.url), "utf8"));

const tsc = spawnSync("node_modules/.bin/tsc", ["--noEmit", "-p", "tsconfig.json"], { encoding: "utf8" });
const own = (tsc.stdout + tsc.stderr).split("\n").filter((l) => l.startsWith("src/lib/admin/"));
assert.deepEqual(own, [], `tsc errors in src/lib/admin:\n${own.join("\n")}`);

const oneOf = (v, set, what) => assert.ok(set.includes(v), `${what}: ${JSON.stringify(v)} not in ${set}`);
const segments = ["new", "regular", "power", "dormant", "at_risk", "never_watched"];
const severities = ["low", "medium", "high", "critical"];
const statuses = ["new", "in_progress", "resolved"];

oneOf(real("me.token").data.via, ["token", "session"], "me.via");
oneOf(real("me.session").data.via, ["token", "session"], "me.via");
assert.equal(real("overview").data.heatmap.weekday_start, "monday");
assert.equal(real("overview").data.heatmap.hours_tz, "UTC");
for (const f of ["catalog", "catalog.movie", "catalog.empty"]) {
  const d = real(f).data;
  oneOf(d.format, ["tv", "movie", "ova", "all"], "catalog.format");
  for (const p of d.quadrant.points) oneOf(p.quadrant_key, ["safe", "push", "watch", "retire"], "quadrant_key");
}
for (const f of ["users.session", "users.token"]) for (const u of real(f).data.users) oneOf(u.segment, segments, "users.segment");
for (const f of ["user-detail.session", "user-detail.token", "user-detail.empty.session"]) oneOf(real(f).data.segment, segments, "detail.segment");
for (const f of ["users-summary", "users-summary.token"]) for (const s of real(f).data.segments) oneOf(s.key, segments, "summary.segment");
for (const f of ["issues", "issue", "issue-create.response", "issue-create-minimal.response", "issue-update.response"]) {
  const d = real(f).data;
  for (const i of d.items ?? [d]) {
    oneOf(i.severity, severities, "issue.severity");
    oneOf(i.status, statuses, "issue.status");
  }
}
for (const f of ["issue-create.request", "issue-create-minimal.request"]) oneOf(real(f).severity, severities, "request.severity");
if (real("issue-update.request").status) oneOf(real("issue-update.request").status, statuses, "request.status");
for (const f of ["growth", "growth.empty"]) {
  assert.deepEqual(real(f).data.visitor_to_signup, { measured: false, value: null });
  assert.deepEqual(real(f).data.acquisition_sources, { measured: false, value: null });
}
console.log("admin types: OK");
