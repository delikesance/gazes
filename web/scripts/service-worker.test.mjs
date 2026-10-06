// public/sw.js in a stubbed worker scope: hashed assets are cached, and the cache stays bounded
// across deploys (each build ships new chunk names; old ones must not pile up forever).
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import vm from "node:vm";

const source = await readFile(path.resolve(import.meta.dirname, "../public/sw.js"), "utf8");

function worker() {
  const store = new Map(); // insertion-ordered, like the Cache API's keys()
  const cache = {
    match: async (req) => store.get(req.url),
    put: async (req, res) => { store.delete(req.url); store.set(req.url, res); },
    keys: async () => [...store.keys()].map((url) => ({ url })),
    delete: async (req) => store.delete(req.url),
  };
  const listeners = {};
  const self = { location: { origin: "https://gazes.test" }, addEventListener: (type, fn) => { listeners[type] = fn; }, skipWaiting() {}, clients: { claim: async () => {} } };
  const context = vm.createContext({ self, URL, caches: { open: async () => cache, keys: async () => [], delete: async () => true }, fetch: async () => ({ ok: true, clone() { return this; } }) });
  vm.runInContext(source, context);
  const get = async (url) => {
    let pending;
    const background = [];
    listeners.fetch({ request: { method: "GET", url }, respondWith: (p) => { pending = p; }, waitUntil: (p) => background.push(p) });
    const res = pending && (await pending);
    await Promise.all(background);
    return res;
  };
  return { store, get };
}

test("hashed build assets are cached; pages and the API are not", async () => {
  const { store, get } = worker();
  await get("https://gazes.test/_next/static/chunks/a1.js");
  await get("https://gazes.test/api/v1/catalog/trending");
  await get("https://gazes.test/anime/1");
  assert.deepEqual([...store.keys()], ["https://gazes.test/_next/static/chunks/a1.js"]);
});

test("the cache is bounded: the oldest assets are evicted", async () => {
  const { store, get } = worker();
  for (let i = 0; i < 1000; i++) await get(`https://gazes.test/_next/static/chunks/${i}.js`);
  assert.ok(store.size <= 300, `${store.size} entries kept`);
  assert.ok(store.has("https://gazes.test/_next/static/chunks/999.js"), "the newest asset was evicted");
  assert.ok(!store.has("https://gazes.test/_next/static/chunks/0.js"), "the oldest asset was kept");
});
