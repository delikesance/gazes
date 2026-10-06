// Minimal service worker: installability plus cached static assets. Never touches the API or video streams.
const STATIC = "gazes-static-v1";
// Each deploy ships new chunk names: keep the newest entries only, so old builds do not pile up.
const MAX_ENTRIES = 300;
async function trim(cache) {
  const keys = await cache.keys(); // insertion order: oldest first
  await Promise.all(keys.slice(0, Math.max(0, keys.length - MAX_ENTRIES)).map((key) => cache.delete(key)));
}
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== STATIC).map((k) => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);
  // Hashed build assets are immutable: serve from cache, fill on first use.
  if (request.method !== "GET" || url.origin !== self.location.origin || !url.pathname.startsWith("/_next/static/")) return;
  event.respondWith(caches.open(STATIC).then(async (cache) => {
    const hit = await cache.match(request);
    if (hit) return hit;
    const res = await fetch(request);
    // Store and trim after answering: the page does not wait on cache bookkeeping.
    if (res.ok) event.waitUntil(cache.put(request, res.clone()).then(() => trim(cache)).catch(() => {}));
    return res;
  }));
});
