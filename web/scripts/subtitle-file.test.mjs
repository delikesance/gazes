// Subtitle files picked by the viewer: SRT/VTT become ASS, ASS passes through, anything else is refused.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "subtitle-file.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/subtitle-file.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { subtitleFileToAss, parseCueTime } = createRequire(import.meta.url)(outfile);

test("cue timestamps parse with comma or dot, with or without hours", () => {
  assert.equal(parseCueTime("00:01:02,500"), 62.5);
  assert.equal(parseCueTime("01:02.500"), 62.5);
  assert.equal(parseCueTime("1:02:03.5"), 3723.5);
  assert.ok(Number.isNaN(parseCueTime("soon")));
});

test("an SRT becomes ASS dialogue with tags and line breaks kept", () => {
  const srt = "﻿1\r\n00:00:01,000 --> 00:00:02,500\r\n<i>Hello</i>\r\nworld\r\n\r\n2\r\n00:01:00,000 --> 00:01:01,000\r\nBye {x}\r\n";
  const ass = subtitleFileToAss("ep.srt", srt);
  assert.ok(ass.includes("[V4+ Styles]") && ass.includes("[Events]"));
  assert.ok(ass.includes("Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,{\\i1}Hello{\\i0}\\Nworld"));
  assert.ok(ass.includes("Dialogue: 0,0:01:00.00,0:01:01.00,Default,,0,0,0,,Bye (x)"));
});

test("WebVTT headers, cue settings and identifiers are tolerated", () => {
  const vtt = "WEBVTT\n\nintro\n00:01.000 --> 00:02.000 align:start\nHi\n\nNOTE x\n\n00:03.000 --> 00:02.000\nbackwards\n";
  const ass = subtitleFileToAss("ep.vtt", vtt);
  assert.equal(ass.split("\n").filter((l) => l.startsWith("Dialogue:")).length, 1);
  assert.ok(ass.includes("0:00:01.00,0:00:02.00,Default,,0,0,0,,Hi"));
});

test("ASS passes through only when it has events; other files and empty cue lists are refused", () => {
  const ass = "[Script Info]\nPlayResY: 480\n\n[Events]\nFormat: Layer, Start\n";
  assert.equal(subtitleFileToAss("x.ass", ass), ass);
  assert.equal(subtitleFileToAss("x.ass", "just text"), null);
  assert.equal(subtitleFileToAss("x.txt", "1\n00:00:01,000 --> 00:00:02,000\nhi"), null);
  assert.equal(subtitleFileToAss("x.srt", "no cues here"), null);
});
