"use client";
import Link from "next/link";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { AnimeCatalogItem, Franchise } from "@/types/api";
import { EpisodeCard } from "./EpisodeCard";
import { LazyImage } from "./ui/LazyImage";

const plain = (html?: string) => (html || "").replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim();

export function SeasonPage({ franchise, season, seasonId, base, resume }: { franchise: Franchise; season?: AnimeCatalogItem; seasonId: number; base: string; resume: { episode: number } | null }) {
  const { t } = useI18n();
  const selected = franchise.seasons.find((s) => s.id === seasonId);
  const seasonURL = `${base}/seasons/${seasonId}`;
  const name = t(selected?.season_name || season?.display_title || "Saison");
  const list = season?.episode_list || [];
  const released = list.filter((e) => !e.upcoming);
  const first = released[0];
  const description = plain(season?.description);
  const total = season?.episodes || list.length;
  const countLabel = (n: number) => t(n === 1 ? "{count} épisode" : "{count} épisodes", { count: n });
  const siblings = franchise.seasons.filter((s) => s.group === (selected?.group || "main"));
  return <div className="season-page">
    <section className="season-intro">
      <div className="season-intro-poster"><LazyImage src={season?.poster_image || selected?.poster_image || franchise.poster_image} alt="" aspectRatio="" priority className="season-intro-art" /></div>
      <div className="season-intro-copy">
        <Link href={base} className="season-back">← {franchise.title}</Link>
        <h1>{name}</h1>
        <p className="series-meta">
          {(season?.season_year || selected?.season_year) && <span>{season?.season_year || selected?.season_year}</span>}
          {total > 0 && <span>{countLabel(total)}</span>}
        </p>
      </div>
      <div className="hero-actions">
        {resume ? <Link className="design-button" href={`${seasonURL}/episodes/${resume.episode}`}><Play size={14} aria-hidden="true" fill="currentColor" />&nbsp;&nbsp;{t("Reprendre")} · {t("Épisode")} {resume.episode}</Link>
          : first && <Link className="design-button" href={`${seasonURL}/episodes/${first.episode_number}`}><Play size={14} aria-hidden="true" fill="currentColor" />&nbsp;&nbsp;{t("Regarder")}</Link>}
        {resume && first && <Link className="design-button secondary-button" href={`${seasonURL}/episodes/${first.episode_number}`}>{t("Depuis le début")}</Link>}
      </div>
    </section>
    {siblings.length > 1 && <nav className="season-tabs" aria-label={t("Saisons")}>{siblings.map((s) => <Link key={s.id} href={`${base}/seasons/${s.id}`} aria-current={s.id === seasonId ? "page" : undefined} className="season-tab">{t(s.season_name)}</Link>)}</nav>}
    <div className="season-body">
      <section id="episodes" className="season-episodes">
        <div className="section-heading"><h2>{t("Épisodes")}</h2><span className="heading-line" /></div>
        <div className="episode-list">{list.map((episode) => <EpisodeCard key={episode.episode_number} seasonId={seasonId} episode={episode} current={resume?.episode === episode.episode_number} href={`${seasonURL}/episodes/${episode.episode_number}`} />)}</div>
        {!list.length && <p className="detail-notice">{t("La liste des épisodes n’est pas encore disponible.")}</p>}
      </section>
      <aside className="season-aside">
        <h2>{t("À propos de la saison")}</h2>
        {description && <p>{description}</p>}
        <dl>
          {(season?.season_year || selected?.season_year) && <><dt>{t("Année")}</dt><dd>{season?.season_year || selected?.season_year}</dd></>}
          {total > 0 && <><dt>{t("Épisodes")}</dt><dd>{total}</dd></>}
          {selected?.status === "NOT_YET_RELEASED" && <><dt>{t("Statut")}</dt><dd>{t("À venir")}</dd></>}
        </dl>
      </aside>
    </div>
  </div>;
}
