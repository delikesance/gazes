"use client";
/* eslint-disable @next/next/no-img-element -- poster URLs come from the catalog CDN */
import Link from "next/link";
import { useEffect, useState } from "react";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listProgress, type SavedProgress } from "@/lib/watch-progress";
import { getSeason } from "@/lib/api";
import { PageGrid } from "@/components/ui/PageGrid";
import { Scribble } from "@/components/ui/Scribble";

function clock(seconds: number): string {
  const total = Math.floor(seconds), h = Math.floor(total / 3600), m = Math.floor((total % 3600) / 60), s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`;
}

type Info = { title: string; poster?: string };
const infoCache = new Map<number, Info>();

export default function HistoryPage() {
  const { t, locale } = useI18n();
  const [items, setItems] = useState<SavedProgress[] | null>(null);
  const [info, setInfo] = useState<Record<number, Info>>({});

  useEffect(() => {
    const refresh = () => setItems(listProgress());
    refresh();
    window.addEventListener("gazes-progress-change", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("gazes-progress-change", refresh); window.removeEventListener("storage", refresh); };
  }, []);

  // Entries saved before titles were stored (or from another device) are completed from the catalog.
  useEffect(() => {
    if (!items) return;
    const controller = new AbortController();
    for (const item of items) {
      if (infoCache.has(item.season)) continue;
      getSeason(item.animeId || item.season, item.season, controller.signal).then((season) => {
        const value = { title: season.display_title, poster: season.poster_image };
        infoCache.set(item.season, value);
        setInfo((current) => ({ ...current, [item.season]: value }));
      }).catch(() => {});
    }
    return () => controller.abort();
  }, [items]);

  return (
    <main className="history-page">
      <PageGrid />
      <Scribble shape="b" width={380} rotate={-6} style={{ right: 40, top: 120 }} />
      <div className="history-inner page-inset">
        <span className="eyebrow">{t("Historique")}</span>
        <h1 className="serif">{t("Reprendre la lecture")}</h1>
        {items && items.length === 0 && <p className="history-empty">{t("Rien à reprendre pour l’instant.")}</p>}
        <ul className="history-list">
          {(items || []).map((item) => {
            const meta = info[item.season] ?? infoCache.get(item.season);
            const title = item.title || meta?.title || t("Anime");
            return (
              <li key={item.season}>
                <Link href={`/anime/${item.animeId || item.season}/seasons/${item.season}/episodes/${item.episode}`} className="history-row">
                  <span className="history-thumb" aria-hidden="true">
                    {meta?.poster && <img src={meta.poster} alt="" loading="lazy" />}
                    <span className="history-play"><Play size={12} /></span>
                  </span>
                  <span className="history-title">{title}</span>
                  <span className="chip">{t("Épisode")} {item.episode} · {clock(item.position)}</span>
                  {item.updatedAt > 1e9 && <span className="history-date">{new Date(item.updatedAt * 1000).toLocaleDateString(locale)}</span>}
                </Link>
              </li>
            );
          })}
        </ul>
      </div>
    </main>
  );
}
