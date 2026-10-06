"use client";
import Link from "next/link";
import { useI18n } from "@/lib/i18n";
import { GENRES, genreLabel, genreSlug } from "@/lib/genres";

export function GenreHeader({ genre }: { genre: string }) {
  const { t } = useI18n();
  return <header className="page-inset genre-header">
    <span className="eyebrow">{t("Genre")}</span>
    <h1 className="serif">{t("Animes {genre} en streaming", { genre: genreLabel(genre) })}</h1>
    <nav aria-label={t("Genres")} className="genre-chips">
      {GENRES.map(([value, label]) => <Link key={value} href={`/genre/${genreSlug(value)}`} className="chip" aria-current={value === genre ? "page" : undefined}>{label}</Link>)}
    </nav>
  </header>;
}
