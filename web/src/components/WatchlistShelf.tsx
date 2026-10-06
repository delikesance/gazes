"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { CalendarPlus, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getFranchise } from "@/lib/api";
import { removeFromWatchlist, useWatchlist } from "@/lib/watchlist";
import { watchlistCalendar } from "@/lib/watchlist-ics";
import { LazyImage } from "./ui/LazyImage";

type Info = { title: string; poster?: string };
const infoCache = new Map<number, Info>();

/** Home shelf of the anime saved to "ma liste". */
export function WatchlistShelf() {
  const { t } = useI18n();
  const saved = useWatchlist();
  const ids = saved.slice(0, 12);
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

  const [exportState, setExportState] = useState<"idle" | "busy" | "empty" | "error">("idle");
  async function exportCalendar() {
    setExportState("busy");
    try {
      // The whole list, not just the shelf's first posters.
      const ics = await watchlistCalendar(saved, window.location.origin);
      if (!ics) { setExportState("empty"); return; }
      const url = URL.createObjectURL(new Blob([ics], { type: "text/calendar" }));
      const link = Object.assign(document.createElement("a"), { href: url, download: "gazes-ma-liste.ics" });
      link.click();
      // Revoking in the same task can cancel the download in some browsers.
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
      setExportState("idle");
    } catch { setExportState("error"); }
  }

  if (!ids.length) return null;
  return <section className="continue-watching" aria-labelledby="watchlist-heading">
    <div className="section-heading"><div className="section-title"><span className="eyebrow">{t("À voir plus tard")}</span><h2 id="watchlist-heading" className="serif">{t("Ma liste")}</h2></div>
      <button type="button" className="clay clay-secondary clay-sm" onClick={exportCalendar} disabled={exportState === "busy"}><CalendarPlus size={16} aria-hidden="true" />{t("Ajouter les sorties à mon calendrier")}</button>
    </div>
    {(exportState === "empty" || exportState === "error") && <p role="status" className="catalog-message">{t(exportState === "empty" ? "Aucune sortie prévue dans les 6 prochaines semaines." : "Impossible de générer le calendrier pour l’instant.")}</p>}
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
