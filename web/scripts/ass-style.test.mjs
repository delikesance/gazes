// Viewer subtitle preferences applied to the ASS style table.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "ass-style.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/ass-style.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { styleAss, DEFAULT_SUBTITLE_STYLE } = createRequire(import.meta.url)(outfile);

const SCRIPT = [
  "[Script Info]", "PlayResY: 720", "",
  "[V4+ Styles]",
  "Format: Name, Fontname, Fontsize, PrimaryColour, Bold, MarginL, MarginR, MarginV, Encoding",
  "Style: Default,Arial,40,&H00FFFFFF,0,10,10,30,1",
  "Style: Top,Arial,20,&H00FFFFFF,0,10,10,12,1", "",
  "[Events]", "Format: Layer, Start, End, Style, Text",
  "Dialogue: 0,0:00:01.00,0:00:02.00,Default,{\\fs99}Hello, world", "",
].join("\n");

test("the default style leaves the script untouched", () => {
  assert.equal(styleAss(SCRIPT, DEFAULT_SUBTITLE_STYLE), SCRIPT);
});

test("font size scales and the bottom margin grows by a share of PlayResY", () => {
  const out = styleAss(SCRIPT, { scale: 1.5, lift: 10 }).split("\n");
  assert.equal(out.find((l) => l.startsWith("Style: Default")), "Style: Default,Arial,60,&H00FFFFFF,0,10,10,102,1");
  assert.equal(out.find((l) => l.startsWith("Style: Top")), "Style: Top,Arial,30,&H00FFFFFF,0,10,10,84,1");
});

test("dialogue lines and other sections are not rewritten", () => {
  const out = styleAss(SCRIPT, { scale: 1.25, lift: 5 });
  assert.ok(out.includes("Dialogue: 0,0:00:01.00,0:00:02.00,Default,{\\fs99}Hello, world"));
  assert.ok(out.includes("PlayResY: 720"));
});

test("without PlayResY the libass default of 288 is used, CRLF is kept, and unrelated text passes through", () => {
  const crlf = "[V4+ Styles]\r\nFormat: Name, Fontsize, MarginV\r\nStyle: Default,20,10\r\n";
  assert.equal(styleAss(crlf, { scale: 1, lift: 10 }), "[V4+ Styles]\r\nFormat: Name, Fontsize, MarginV\r\nStyle: Default,20,39\r\n");
  assert.equal(styleAss("WEBVTT\n\n00:01.000 --> 00:02.000\nhi", { scale: 2, lift: 5 }), "WEBVTT\n\n00:01.000 --> 00:02.000\nhi");
});
