// Cover art is routed through the backend's disk cache; anything else is left alone.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const outfile = path.join(root, "node_modules", ".cache", "image-url.cjs");
await build({ absWorkingDir: root, entryPoints: ["src/lib/image.ts"], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", logLevel: "silent" });
const { cachedImage } = createRequire(import.meta.url)(outfile);
delete process.env.NEXT_PUBLIC_API_BASE;

const cover = "https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/bx1-abc.jpg";

test("AniList art goes through the same-origin cache", () => {
  assert.equal(cachedImage(cover), `/api/v1/img?u=${encodeURIComponent(cover)}`);
});

test("other hosts, relative and malformed values are returned unchanged", () => {
  for (const url of ["https://img1.ak.crunchyroll.com/x.jpg", "/api/v1/catalog/seasons/1/episodes/1/preview", "data:image/png;base64,AAAA", "http://s4.anilist.co/x.jpg", "https://s4.anilist.co.evil.example/x.jpg", "not a url"]) {
    assert.equal(cachedImage(url), url, url);
  }
});

test("empty values stay empty", () => {
  assert.equal(cachedImage(undefined), undefined);
  assert.equal(cachedImage(null), null);
  assert.equal(cachedImage(""), "");
});

test("an already cached URL is not wrapped twice", () => {
  const once = cachedImage(cover);
  assert.equal(cachedImage(once), once);
});
