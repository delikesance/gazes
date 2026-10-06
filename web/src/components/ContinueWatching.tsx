"use client";
/* eslint-disable @next/next/no-img-element -- poster URLs come from the catalog CDN */
import Link from "next/link";
import { useEffect, useState } from "react";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listProgress, type SavedProgress } from "@/lib/watch-progress";
import { getSeason } from "@/lib/api";

const LIMIT = 6;
const posterCache = new Map<number, { title: string; poster?: string }>();

/** Shelf of the latest unfinished episodes, read from this device's (or the account's) watch progress. */
export function ContinueWatching() {
  const { t } = useI18n();
  const [items, setItems] = useState<SavedProgress[]>([]);
  const [, bump] = useState(0);

  useEffect(() => {
    const refresh = () => setItems(listProgress().slice(0, LIMIT));
    refresh();
    window.addEventListener("gazes-progress-change", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("gazes-progress-change", refresh); window.removeEventListener("storage", refresh); };
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    for (const item of items) {
      if (posterCache.has(item.season)) continue;
      getSeason(item.animeId || item.season, item.season, controller.signal).then((season) => {
        posterCache.set(item.season, { title: season.display_title, poster: season.poster_image });
        bump((n) => n + 1);
      }).catch(() => {});
    }
    return () => controller.abort();
  }, [items]);

  if (!items.length) return null;
  return <section className="continue-watching" aria-labelledby="continue-heading">
    <div className="section-heading"><div className="section-title"><span className="eyebrow">{t("Historique")}</span><h2 id="continue-heading" className="serif">{t("Reprendre la lecture")}</h2></div><Link href="/history" className="clay clay-secondary clay-sm">{t("Tout voir")}</Link></div>
    <ul className="history-grid page-inset">
      {items.map((item) => {
        const meta = posterCache.get(item.season);
        const title = item.title || meta?.title || t("Anime");
        const watched = item.duration ? Math.min(100, Math.max(0, Math.round((item.position / item.duration) * 100))) : 100;
        return <li key={item.season}>
          <Link href={`/anime/${item.animeId || item.season}/seasons/${item.season}/episodes/${item.episode}`} className="history-card">
            <span className="history-poster" role="img" aria-label={`${title}, ${watched} %`}>
              {meta?.poster && <img src={meta.poster} alt="" loading="lazy" />}
              {meta?.poster && watched < 100 && <img className="history-poster-gray" src={meta.poster} alt="" aria-hidden="true" loading="lazy" style={{ clipPath: `inset(0 0 ${watched}% 0)` }} />}
              <span className="history-play"><Play size={18} fill="currentColor" /></span>
            </span>
            <span className="history-meta"><span className="history-episode">{t("Épisode")} {item.episode}</span></span>
            <span className="history-title">{title}</span>
          </Link>
        </li>;
      })}
    </ul>
  </section>;
}
