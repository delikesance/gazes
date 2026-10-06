"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getFranchise } from "@/lib/api";
import { removeFromWatchlist, useWatchlist } from "@/lib/watchlist";
import { LazyImage } from "./ui/LazyImage";

type Info = { title: string; poster?: string };
const infoCache = new Map<number, Info>();

/** Home shelf of the anime saved to "ma liste". */
export function WatchlistShelf() {
  const { t } = useI18n();
  const ids = useWatchlist().slice(0, 12);
  const key = ids.join(",");
  const [, bump] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    for (const id of key ? key.split(",").map(Number) : []) {
      if (infoCache.has(id)) continue;
      getFranchise(id, controller.signal).then((f) => {
        infoCache.set(id, { title: f.title, poster: f.poster_image });
        bump((n) => n + 1);
      }).catch(() => {});
    }
    return () => controller.abort();
  }, [key]);

  if (!ids.length) return null;
  return <section className="continue-watching" aria-labelledby="watchlist-heading">
    <div className="section-heading"><div className="section-title"><span className="eyebrow">{t("À voir plus tard")}</span><h2 id="watchlist-heading" className="serif">{t("Ma liste")}</h2></div></div>
    <ul className="poster-grid page-inset watchlist-grid">
      {ids.map((id) => {
        const info = infoCache.get(id);
        const title = info?.title || t("Anime");
        return <li key={id} className="poster-cell">
          <Link href={`/anime/${id}`} className="poster-card" title={title}>
            <LazyImage src={info?.poster} alt={title} aspectRatio="" className="poster-art" />
            <div className="poster-overlay"><div><h3>{title}</h3></div></div>
          </Link>
          <button type="button" className="poster-hide" onClick={() => removeFromWatchlist(id)} aria-label={t("Retirer {title} de ma liste", { title })} title={t("Retirer de ma liste")}><X size={16} aria-hidden="true" /></button>
        </li>;
      })}
    </ul>
  </section>;
}
