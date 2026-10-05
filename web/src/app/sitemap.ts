import type { MetadataRoute } from "next";
import { getCatalogPopular, getCatalogSeasonal } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

export const revalidate = 3600;

const PAGES = 3;
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

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const ids = await collectIds();
  return [
    { url: SITE_URL, changeFrequency: "daily", priority: 1 },
    ...ids.map((id) => ({ url: `${SITE_URL}/anime/${id}`, changeFrequency: "weekly" as const, priority: 0.7 })),
  ];
}
