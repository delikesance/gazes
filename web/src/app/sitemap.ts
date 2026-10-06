import type { MetadataRoute } from "next";
import { getCatalogPopular, getCatalogSeasonal } from "@/lib/api";
import { GENRES, genreSlug } from "@/lib/genres";
import { SITE_URL } from "@/lib/site";
import { CHANGELOG } from "@/lib/changelog";

// Rendered on request, never at build time: the build has no backend, so a prerendered sitemap would
// freeze on the home page alone. The catalog lookups are cached in memory instead.
export const dynamic = "force-dynamic";

const TTL_MS = 3600_000;
let cache: { ids: number[]; at: number } | null = null;

const PAGES = 10;
const PER_PAGE = 50;

async function collectIds(): Promise<number[]> {
  const ids = new Set<number>();
  for (const fetchPage of [getCatalogSeasonal, getCatalogPopular]) {
    for (let page = 1; page <= PAGES; page++) {
      try {
        const res = await fetchPage(page, PER_PAGE);
        for (const item of res.items) ids.add(item.id);
        if (!res.has_next_page) break;
      } catch (e) {
        console.error("sitemap fetch error:", e);
        break;
      }
    }
  }
  return [...ids];
}

async function cachedIds(): Promise<number[]> {
  if (cache && Date.now() - cache.at < TTL_MS) return cache.ids;
  const ids = await collectIds();
  // An empty result means the backend or AniList failed: serve it, but retry on the next request.
  if (ids.length) cache = { ids, at: Date.now() };
  return ids.length ? ids : cache?.ids ?? [];
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const ids = await cachedIds();
  const lastModified = new Date();
  return [
    { url: SITE_URL, lastModified, changeFrequency: "daily", priority: 1 },
    { url: `${SITE_URL}/soutenir`, changeFrequency: "monthly", priority: 0.2 },
    { url: `${SITE_URL}/changelog`, lastModified: new Date(`${CHANGELOG[0].date}T00:00:00Z`), changeFrequency: "weekly", priority: 0.3 },
    ...GENRES.map(([value]) => ({ url: `${SITE_URL}/genre/${genreSlug(value)}`, lastModified, changeFrequency: "weekly" as const, priority: 0.6 })),
    ...ids.map((id) => ({ url: `${SITE_URL}/anime/${id}`, lastModified, changeFrequency: "weekly" as const, priority: 0.7 })),
  ];
}
