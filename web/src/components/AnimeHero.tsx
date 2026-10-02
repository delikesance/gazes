"use client";
import { useI18n } from "@/lib/i18n";
import Link from "next/link";
import { LazyImage } from "./ui/LazyImage";
import type { ReactNode } from "react";

interface Props {
  title: string;
  banner?: string;
  poster?: string;
  episodes?: number;
  year?: number;
  children?: ReactNode;
}

export function AnimeHero({ title, banner, poster, episodes, year, children }: Props) {
  const { t } = useI18n();
  return <section className="anime-hero" aria-label={t("À la une")}>
    <div className="anime-hero-art" aria-hidden="true">
      <LazyImage key={`wide:${banner}:${poster}`} src={banner || poster} fallbackSrc={poster} alt="" priority aspectRatio="" className="hero-wide" />
      <LazyImage key={`portrait:${poster}:${banner}`} src={poster || banner} fallbackSrc={banner} alt="" priority aspectRatio="" className="hero-portrait" />
    </div>
    <div className="anime-hero-shade" />
    <div className="anime-hero-copy">
      {(episodes || year) && <p className="hero-meta">{episodes ? <span>{t(episodes === 1 ? "{count} épisode" : "{count} épisodes", {count:episodes})}</span> : null}{episodes && year ? <span aria-hidden="true">·</span> : null}{year && <span>{year}</span>}</p>}
      <h1>{title}</h1>
      {children && <div className="hero-actions">{children}</div>}
    </div>
  </section>;
}

export function FeaturedAnime({ anime }: { anime: import("@/types/api").AnimeCatalogItem }) {
  const { t } = useI18n();
  const upcoming = anime.status === "NOT_YET_RELEASED";
  return <AnimeHero title={anime.title_romaji || anime.display_title} banner={anime.banner_image} poster={anime.media_poster_image || anime.poster_image} episodes={anime.episodes} year={anime.season_year}>
    {upcoming ? <span className="design-button muted-button">{t("Bientôt disponible")}</span> : <Link className="design-button" href={`/anime/${anime.id}/seasons/${anime.media_id || anime.id}/episodes/1`}>{t("Regarder")}</Link>}
    <Link className="design-button secondary-button" href={`/anime/${anime.id}`}>{t("En savoir plus")}</Link>
  </AnimeHero>;
}
