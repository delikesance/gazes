"use client";
import { useI18n } from "@/lib/i18n";
import type { AnimeCatalogItem } from "@/types/api";
import { LazyImage } from "./ui/LazyImage";
import Link from "next/link";
import { Play, X } from "lucide-react";

interface Props {
  anime: AnimeCatalogItem;
  seasonal?: boolean;
  onSelect?: (anime: AnimeCatalogItem) => void;
  onQuickPlayEp1?: (anime: AnimeCatalogItem) => void;
  /** Shows a "pas intéressé" button (suggestions only). */
  onHide?: (anime: AnimeCatalogItem) => void;
}

export function AnimeCatalogCard({ anime, onSelect, onHide, seasonal = false }: Props) {
  const { t, locale } = useI18n();
  const title = seasonal ? anime.media_title || anime.title_romaji || anime.title_english || anime.display_title : anime.franchise_title || anime.display_title;
  const upcoming = anime.status === "NOT_YET_RELEASED";
  const [year, month, day] = (anime.start_date || "").split("-").map(Number);
  const premiere = year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= 31
    ? new Intl.DateTimeFormat(locale === "fr" ? "fr-FR" : "en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" }).format(new Date(Date.UTC(year, month - 1, day)))
    : null;
  const releaseMonth = year > 0 && month >= 1 && month <= 12
    ? new Intl.DateTimeFormat(locale === "fr" ? "fr-FR" : "en-GB", { month: "long", year: "numeric", timeZone: "UTC" }).format(new Date(Date.UTC(year, month - 1, 1)))
    : null;
  const href = seasonal ? `/anime/${anime.id}/seasons/${anime.media_id || anime.id}` : `/anime/${anime.id}`;
  const card = <Link href={href} className={`poster-card ${seasonal ? "seasonal-card" : ""}`} title={title}
    onClick={event => { if (onSelect && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) { event.preventDefault(); onSelect(anime); } }}>
    <LazyImage src={seasonal ? anime.media_poster_image || anime.poster_image : anime.poster_image} alt={title} aspectRatio="" className="poster-art" />
    <div className="poster-overlay">
      <span className="poster-play" aria-hidden="true"><Play size={20} fill="currentColor" /></span>
      <div>{upcoming ? <p className="premiere-date">{premiere ? <>{t("Épisode 1 ·")}{" "}<time dateTime={anime.start_date}>{premiere}</time></> : releaseMonth ? t("Prévu en {date}", {date:releaseMonth}) : year > 0 ? t("Prévu en {date}", {date:year}) : t("Épisode 1 · Date à confirmer")}</p> : <p className="availability-meta">{anime.available_episodes !== undefined ? t(anime.available_episodes === 1 ? "{count} épisode disponible" : "{count} épisodes disponibles", {count:anime.available_episodes}) : t("Disponibilité à confirmer")}</p>}<h3>{title}</h3></div>
    </div>
  </Link>;
  if (!onHide) return card;
  return <div className="poster-cell">
    {card}
    <button type="button" className="poster-hide" onClick={() => onHide(anime)} aria-label={t("Pas intéressé par {title}", { title })} title={t("Pas intéressé")}><X size={16} aria-hidden="true" /></button>
  </div>;
}
