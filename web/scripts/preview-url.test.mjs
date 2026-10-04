// The episode preview URL ends up in server-rendered markup that the browser fetches, so it must never carry the
// docker-internal BACKEND_URL host that getApiBase() returns during SSR.
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const root = path.resolve(import.meta.dirname, "..");
const bundle = async (entry, name) => {
  const outfile = path.join(root, "node_modules", ".cache", `${name}.cjs`);
  await build({ absWorkingDir: root, entryPoints: [entry], outfile, bundle: true, format: "cjs", platform: "node", packages: "external", jsx: "automatic", logLevel: "silent" });
  return createRequire(import.meta.url)(outfile);
};
const api = await bundle("src/lib/api.ts", "preview-url-api");
const cardModule = await bundle("src/components/EpisodeCard.tsx", "preview-url-card");
const { renderToString } = createRequire(path.join(root, "package.json"))("react-dom/server");
const React = createRequire(path.join(root, "package.json"))("react");

process.env.BACKEND_URL = "http://backend:8090";
delete process.env.NEXT_PUBLIC_API_BASE;
assert.equal(typeof window, "undefined");

test("episodePreviewUrl stays same-origin during SSR", () => {
  const url = api.episodePreviewUrl(21459, 2);
  assert.equal(url, "/api/v1/catalog/seasons/21459/episodes/2/preview");
  assert.ok(!url.includes("backend"));
});

test("episodePreviewUrl honours NEXT_PUBLIC_API_BASE", () => {
  process.env.NEXT_PUBLIC_API_BASE = "https://api.example.test/api/v1";
  try {
    assert.equal(api.episodePreviewUrl(1, 3), "https://api.example.test/api/v1/catalog/seasons/1/episodes/3/preview");
  } finally {
    delete process.env.NEXT_PUBLIC_API_BASE;
  }
});

test("server-rendered EpisodeCard markup has no internal backend host", () => {
  const episode = { episode_number: 1, title: "Episode 1 - Start", thumbnail: "", upcoming: false };
  const html = renderToString(React.createElement(cardModule.EpisodeCard, { episode, href: "/x", seasonId: 21459 }));
  assert.ok(html.includes('src="/api/v1/catalog/seasons/21459/episodes/1/preview"'), html);
  assert.ok(!html.includes("http://backend"), html);
});

