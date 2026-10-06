"use client";
import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { listProgress, resumeTarget, type SavedProgress } from "@/lib/watch-progress";
import { cachedSeasonInfo, loadSeasonInfo } from "@/lib/season-info";

export type Resume = { href: string; title: string; episode: number; next: boolean; key: string };

/** The latest unfinished episode (or the one after a finished one), kept in step with this device's and the account's progress. */
export function useResume(): Resume | null {
  const { t } = useI18n();
  const [item, setItem] = useState<SavedProgress | null>(null);
  const [, bump] = useState(0);
  useEffect(() => {
    const refresh = () => setItem(listProgress()[0] ?? null);
    refresh();
    window.addEventListener("gazes-progress-change", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("gazes-progress-change", refresh); window.removeEventListener("storage", refresh); };
  }, []);
  useEffect(() => {
    if (!item || cachedSeasonInfo(item.season)) return;
    const controller = new AbortController();
    loadSeasonInfo(item.animeId, item.season, controller.signal).then(() => bump((n) => n + 1)).catch(() => {});
    return () => controller.abort();
  }, [item]);
  if (!item) return null;
  const info = cachedSeasonInfo(item.season);
  const target = resumeTarget(item, info?.total);
  return {
    href: `/anime/${item.animeId || item.season}/seasons/${item.season}/episodes/${target.episode}`,
    title: item.title || info?.title || t("Anime"),
    episode: target.episode,
    next: target.next,
    key: `${item.season}:${target.episode}`,
  };
}
