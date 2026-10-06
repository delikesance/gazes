"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { Franchise } from "@/types/api";
import { listProgress, resumeTarget, type SavedProgress } from "@/lib/watch-progress";
import { WatchlistButton } from "./WatchlistButton";
import { AnimeNoteButton } from "./AnimeNoteButton";
import { ListMenu } from "./ListMenu";
import { LazyImage } from "./ui/LazyImage";

const plain = (html?: string) => (html || "").replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim();

export function SeriesPage({ franchise, base, warning }: { franchise: Franchise; base: string; warning?: React.ReactNode }) {
  const { t } = useI18n();
  const [expanded, setExpanded] = useState(false);
  const [saved, setSaved] = useState<SavedProgress | null>(null);
  // The most recent unfinished episode of any season of this series: the primary button resumes it.
  useEffect(() => {
    const ids = new Set(franchise.seasons.map((s) => s.id));
    const refresh = () => setSaved(listProgress().find((item) => ids.has(item.season)) ?? null);
    refresh();
    window.addEventListener("gazes-progress-change", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("gazes-progress-change", refresh); window.removeEventListener("storage", refresh); };
  }, [franchise.seasons]);
  const main = franchise.seasons.filter((s) => s.group === "main");
  const first = main[0] || franchise.seasons[0];
  const totalEpisodes = main.reduce((sum, s) => sum + (s.episodes || 0), 0);
  const years = franchise.seasons.map((s) => s.season_year).filter((y): y is number => !!y);
  const start = years.length ? Math.min(...years) : undefined, end = years.length ? Math.max(...years) : undefined;
  const description = plain(franchise.description);
  const count = (n: number | undefined) => n ? t(n === 1 ? "{count} épisode" : "{count} épisodes", { count: n }) : t("Nombre d’épisodes inconnu");
  const groups = ([["main", t("Saisons")], ["movies", t("Films")], ["extras", t("Spéciaux et histoires annexes")]] as const)
    .map(([group, title]) => ({ group, title, entries: franchise.seasons.filter((s) => s.group === group) })).filter((g) => g.entries.length);
  const savedSeason = saved ? franchise.seasons.find((s) => s.id === saved.season) : undefined;
  const target = saved ? resumeTarget(saved, savedSeason?.episodes) : null;
  const resumeLabel = saved && target ? `${savedSeason?.season_number && savedSeason.group === "main" ? `S${savedSeason.season_number} · ` : ""}${t("Épisode")} ${target.episode}` : "";
  return <div className="series-page">
    {(franchise.banner_image || franchise.poster_image) && <div className="series-banner" data-fallback={!franchise.banner_image} aria-hidden="true">
      <LazyImage src={franchise.banner_image || franchise.poster_image} alt="" aspectRatio="" priority className="series-banner-art" />
    </div>}
    <section className="series-intro">
      <div className="series-poster">
        <LazyImage src={franchise.poster_image} alt={franchise.title} aspectRatio="" priority className="series-poster-art" />
      </div>
      <div className="series-copy">
        <h1>{franchise.title}</h1>
        <p className="series-meta">
          {start && <span>{end && end !== start ? `${start} – ${end}` : start}</span>}
          {main.length > 0 && <span>{t(main.length === 1 ? "{count} saison" : "{count} saisons", { count: main.length })}</span>}
          {totalEpisodes > 0 && <span>{count(totalEpisodes)}</span>}
        </p>
        {description && <><p className="series-description" data-expanded={expanded}>{description}</p>
          <button type="button" className="series-more" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>{expanded ? t("Voir moins") : t("Lire la suite")}</button></>}
        {warning}
        <div className="hero-actions">
          {saved ? <Link className="design-button" href={`${base}/seasons/${saved.season}/episodes/${target?.episode ?? saved.episode}`}><Play size={14} aria-hidden="true" fill="currentColor" />&nbsp;&nbsp;{t(target?.next ? "Épisode suivant" : "Reprendre")} · {resumeLabel}</Link>
            : first && first.status !== "NOT_YET_RELEASED" && <Link className="design-button" href={`${base}/seasons/${first.id}/episodes/1`}><Play size={14} aria-hidden="true" fill="currentColor" />&nbsp;&nbsp;{t("Regarder")}</Link>}        <WatchlistButton animeId={franchise.id} /><AnimeNoteButton animeId={franchise.id} /><ListMenu animeId={franchise.id} /></div>
      </div>
    </section>
    <div id="seasons">{groups.map(({ group, title, entries }) => group === "main"
      ? <section key={group} className="detail-section"><div className="section-heading"><h2>{title}</h2><span className="heading-line" /></div>
        <div className="season-grid">{entries.map((s) => <Link key={s.id} href={`${base}/seasons/${s.id}`} className="season-card"><LazyImage src={s.poster_image} alt={s.title} aspectRatio="" className="season-poster" /><h3>{t(s.season_name)}</h3><p>{s.season_year || t("Date inconnue")} · {count(s.episodes)}{s.status === "NOT_YET_RELEASED" ? ` ${t("· À venir")}` : ""}</p></Link>)}</div></section>
      : null)}
    </div>
    {groups.some((g) => g.group !== "main") && <div className="series-extras">{groups.filter((g) => g.group !== "main").map(({ group, title, entries }) => <section key={group} className="detail-section"><div className="section-heading"><h2>{title}</h2><span className="heading-line" /></div>
      <ul className="series-list">{entries.map((s) => <li key={s.id}><Link href={`${base}/seasons/${s.id}`} className="series-row">
        <span className="series-row-thumb"><LazyImage src={s.poster_image} alt="" aspectRatio="" className="series-row-art" /></span>
        <span className="series-row-copy"><span className="series-row-title">{t(s.season_name)}</span><span className="series-row-meta">{s.season_year || t("Date inconnue")} · {count(s.episodes)}{s.status === "NOT_YET_RELEASED" ? ` ${t("· À venir")}` : ""}</span></span>
        <Play size={16} aria-hidden="true" />
      </Link></li>)}</ul></section>)}</div>}
  </div>;
}
