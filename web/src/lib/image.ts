// AniList's CDN is slow from some networks and the picture must survive an outage there: cover and banner
// URLs go through the backend's disk cache (GET /api/v1/img), which fetches each image once and serves it
// as immutable. Mirrors the host allowlist in internal/imagecache; every other URL is left alone.
import { publicApiBase } from "@/lib/api";

const CACHED_HOSTS = new Set(["s4.anilist.co"]);

export function cachedImage<T extends string | null | undefined>(src: T): T | string {
  if (!src) return src;
  let url: URL;
  try {
    url = new URL(src);
  } catch {
    return src; // relative (already ours), data: or malformed
  }
  if (url.protocol !== "https:" || url.username || url.port || !CACHED_HOSTS.has(url.hostname)) return src;
  return `${publicApiBase()}/img?u=${encodeURIComponent(src)}`;
}
