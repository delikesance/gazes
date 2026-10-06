"use client";
import { getSeason } from "./api";

export type SeasonInfo = { title: string; poster?: string; total?: number };
const cache = new Map<number, SeasonInfo>();

export function cachedSeasonInfo(season: number): SeasonInfo | undefined {
  return cache.get(season);
}

/** Title, poster and episode count of a season, fetched once per session (the backend keeps the answer for good). */
export async function loadSeasonInfo(animeId: number, season: number, signal?: AbortSignal): Promise<SeasonInfo> {
  const hit = cache.get(season);
  if (hit) return hit;
  const data = await getSeason(animeId || season, season, signal);
  const info = { title: data.display_title, poster: data.poster_image, total: data.episodes };
  cache.set(season, info);
  return info;
}
