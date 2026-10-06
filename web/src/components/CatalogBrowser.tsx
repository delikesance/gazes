"use client";
import { listProgress } from "@/lib/watch-progress";
import { watchedAnimeIds, listWatchSessions } from "@/lib/watch-log";
import { hideAnime, listHidden } from "@/lib/hidden-anime";
import { genreLabel, parseList } from "@/lib/genres";
import { ErrorAlert } from "./ErrorAlert";
import { errorCode } from "@/lib/error-code";
import { useI18n } from "@/lib/i18n";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { AnimeCatalogCard } from "./AnimeCatalogCard";
import { FeaturedAnimeCarousel } from "./FeaturedAnimeCarousel";
import { WatchlistShelf } from "./WatchlistShelf";
import { ContinueWatching } from "./ContinueWatching";
import { SeasonalGrid } from "./SeasonalGrid";
import { AccountCta } from "./AccountCta";
import { MobileBack } from "./MobileBack";
import { rememberResults } from "@/lib/navigation-history";
import { ReleaseCalendar } from "./ReleaseCalendar";
import type { CatalogResponse } from "@/types/api";
import { getCatalogPopular, getCatalogForYou, getCatalogSeasonal, searchCatalog } from "@/lib/api";

interface CatalogBrowserProps {
  initialData?: CatalogResponse | null;
  initialPopular?: CatalogResponse | null;
  initialQuery?: string;
  initialGenre?: string;
  initialExclude?: string;
  initialTab?: string;
  initialPage?: number;
}

/** Anime ids the viewer watched, most recent first: the seeds of the suggestions feed. */
export function watchedSeeds(): number[] {
  const ids = watchedAnimeIds(5);
  for (const item of listProgress()) {
    const id = item.animeId || item.season;
    if (ids.length < 5 && id > 0 && !ids.includes(id)) ids.push(id);
  }
  return ids;
}

/** The 300 most recent local sessions, trimmed to what the taste profile uses. */
export function recentSessions() {
  return listWatchSessions().slice(-300).map(({ anime_id, season_id, genres, watched_seconds, duration, completed, updated_at }) =>
    ({ anime_id, season_id, genres, watched_seconds, duration, completed, updated_at }));
}

export function CatalogBrowser({
  initialData = null,
  initialPopular = null,
  initialQuery = "",
  initialGenre = "",
  initialExclude = "",
  initialTab = "trending",
  initialPage = 1,
}: CatalogBrowserProps = {}) {
  const { t } = useI18n();
  const router = useRouter();
  const [q, setQ] = useState(initialQuery);
  const [genre, setGenre] = useState(initialGenre);
  const [exclude, setExclude] = useState(initialExclude);
  const [tab, setTab] = useState(initialTab);
  const [page, setPage] = useState(initialPage);
  const [data, setData] = useState<CatalogResponse | null>(initialData);
  const [loading, setLoading] = useState(!initialData);
  const [error, setError] = useState("");
  const [errorCodeValue, setErrorCodeValue] = useState("");
  const [retry, setRetry] = useState(0);
  const [popular, setPopular] = useState<CatalogResponse | null>(initialPopular);
  const [featuredUnavailable, setFeaturedUnavailable] = useState(false);
  const discovery = !q && !genre && !exclude && page === 1;
  // The default discovery tab is the release calendar, which loads its own schedule.
  const isSuggestions = tab === "suggestions" || tab === "popular";
  const needsData = !(discovery && !isSuggestions);

  useEffect(() => {
    if (typeof window !== "undefined") {
      const search = new URLSearchParams(window.location.search);
      setQ(search.get("q") ?? initialQuery ?? "");
      setGenre(search.get("genres") ?? search.get("genre") ?? initialGenre ?? "");
      setExclude(search.get("exclude") ?? initialExclude ?? "");
      setTab(search.get("tab") ?? initialTab ?? "trending");
      setPage(Math.max(1, Number(search.get("page")) || initialPage || 1));
    }
  }, [initialQuery, initialGenre, initialTab, initialPage]);

  function update(values: Record<string, string>, scroll = true, replace = false) {
    const current = typeof window !== "undefined" ? new URLSearchParams(window.location.search) : new URLSearchParams();
    Object.entries(values).forEach(([k, v]) => v ? current.set(k, v) : current.delete(k));
    (replace ? router.replace : router.push)(`/?${current.toString()}`, { scroll });
  }

  // A series page opened from these results offers a way back to them.
  useEffect(() => {
    if (discovery) { rememberResults(null); return; }
    const label = q ? t("Résultats pour « {query} »", { query: q }) : t("Résultats");
    rememberResults(`${window.location.pathname}${window.location.search}`, label.length > 28 ? t("Résultats") : label);
  }, [discovery, q, genre, exclude, t]);

  useEffect(() => {
    if (!needsData) return;
    let active = true;
    setLoading(true);
    setError("");
    const request = q || genre || exclude ? searchCatalog(q, genre, page, 24, exclude) : isSuggestions ? getCatalogForYou(watchedSeeds(), recentSessions(), listHidden(), page, 28) : getCatalogSeasonal(page, 24);
    request.then(result => {
      if (active) {
        setData(result);
        setLoading(false);
        setError("");
      }
    }).catch(err => {
      if (active) {
        setError(err instanceof Error ? err.message : "Impossible de charger le catalogue.");
        setErrorCodeValue(errorCode(err, "CAT"));
        setLoading(false);
      }
    });
    return () => {
      active = false;
    };
  }, [q, genre, exclude, tab, page, retry, needsData, isSuggestions]);

  useEffect(() => {
    if (!discovery) return;
    let active = true;
    getCatalogPopular(1, 12).then(result => {
      if (active) {
        setPopular(result);
        setFeaturedUnavailable(false);
      }
    }).catch(() => {
      if (active) setFeaturedUnavailable(true);
    });
    return () => { active = false; };
  }, [discovery, retry]);

  function hide(anime: { id: number; media_id?: number }) {
    hideAnime(anime.id);
    if (anime.media_id) hideAnime(anime.media_id);
    setData(current => current ? { ...current, items: current.items.filter(item => item.id !== anime.id) } : current);
  }

  const featured = discovery ? (popular?.items || []).filter(anime =>
    anime.status !== "NOT_YET_RELEASED" && (anime.banner_image || anime.media_poster_image || anime.poster_image)) : [];
  const heroLoading = discovery && !popular && !featuredUnavailable;
  const filterSummary = [parseList(genre).map(genreLabel).join(", "), parseList(exclude).length ? t("sans {genres}", { genres: parseList(exclude).map(genreLabel).join(", ") }) : ""].filter(Boolean).join(" · ");
  const cards = data?.items?.map(anime => <AnimeCatalogCard key={anime.media_id || anime.id} anime={anime} seasonal={!q && !genre && !exclude && !isSuggestions} onHide={isSuggestions ? hide : undefined} />);

  return <main className={`catalog-page ${discovery && !error && (featured.length || heroLoading) ? "has-feature" : ""}`}>
    {!discovery && <MobileBack fallback="/" label={t("Catalogue")} />}
    {featured.length > 0 && <FeaturedAnimeCarousel items={featured} />}
    {heroLoading && !error && <div className="hero-skeleton" role="status" aria-label={t("Chargement du catalogue")}><div /><div /></div>}
    {discovery && !error && !heroLoading && <div className="shelves"><ContinueWatching /><WatchlistShelf /></div>}
    {discovery && !error && <section className="season-discovery" aria-label={t("Catalogue")}>
      <div className="catalog-tabs-row page-inset">
        <div className="catalog-tabs" role="group" aria-label={t("Catalogue")}>
          <button type="button" aria-pressed={!isSuggestions} onClick={() => update({ tab: "", page: "1" }, false, true)}>{t("Calendrier")}</button>
          <Link href="/for-you">{t("Suggestions")}</Link>
        </div>
      </div>
      {isSuggestions ? (data && data.items && data.items.length > 0 && <>
        <div className="section-heading"><div className="section-title"><span className="eyebrow">{t("Suggestions")}</span><h2 id="season-heading" className="serif">{t("Pour vous")}</h2></div></div>
        <SeasonalGrid count={data.items.length} evenRows={false}>{cards}</SeasonalGrid>
      </>) : <ReleaseCalendar />}
    </section>}
    {discovery && !error && <AccountCta />}
    {(!discovery || error || (loading && needsData) || (needsData && data && data.items && data.items.length === 0)) && <div className="catalog-tools page-inset">
      {error ? <ErrorAlert className="my-8" message={error} code={errorCodeValue} onRetry={() => setRetry(retry + 1)} /> : (loading && needsData) ? <p role="status" className="catalog-message">{t("Chargement…")}</p> : <>
        {!discovery && <section><h1 className="results-heading">{q ? t("Résultats pour « {query} »", {query:q}) : filterSummary || t("Explorer les animes")}</h1><div className="poster-grid">{cards}</div></section>}
        {data && data.items && data.items.length === 0 && <p className="catalog-message">{t("Aucun anime trouvé.")}</p>}
      </>}
      {!discovery && data && <nav aria-label={t("Pagination")} className="catalog-pagination"><button disabled={page <= 1} onClick={() => update({page:String(page - 1)})}><ArrowLeft size={16} /> {" "}{t("Précédent")}</button><span>{t("Page")}{" "}{page}</span><button disabled={!data?.has_next_page} onClick={() => update({page:String(page + 1)})}>{t("Suivant")}{" "}<ArrowRight size={16} /></button></nav>}
    </div>}
  </main>;
}
