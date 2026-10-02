"use client";
import { ErrorAlert } from "./ErrorAlert";
import { errorCode } from "@/lib/error-code";
import { useI18n } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { AnimeCatalogCard } from "./AnimeCatalogCard";
import { FeaturedAnimeCarousel } from "./FeaturedAnimeCarousel";
import { SeasonalGrid } from "./SeasonalGrid";
import { AccountCta } from "./AccountCta";
import { ReleaseCalendar } from "./ReleaseCalendar";
import type { CatalogResponse } from "@/types/api";
import { getCatalogPopular, getCatalogSeasonal, searchCatalog } from "@/lib/api";

interface CatalogBrowserProps {
  initialData?: CatalogResponse | null;
  initialPopular?: CatalogResponse | null;
  initialQuery?: string;
  initialGenre?: string;
  initialTab?: string;
  initialPage?: number;
}

export function CatalogBrowser({
  initialData = null,
  initialPopular = null,
  initialQuery = "",
  initialGenre = "",
  initialTab = "trending",
  initialPage = 1,
}: CatalogBrowserProps = {}) {
  const { t } = useI18n();
  const router = useRouter();
  const [q, setQ] = useState(initialQuery);
  const [genre, setGenre] = useState(initialGenre);
  const [tab, setTab] = useState(initialTab);
  const [page, setPage] = useState(initialPage);
  const [data, setData] = useState<CatalogResponse | null>(initialData);
  const [loading, setLoading] = useState(!initialData);
  const [error, setError] = useState("");
  const [errorCodeValue, setErrorCodeValue] = useState("");
  const [retry, setRetry] = useState(0);
  const [popular, setPopular] = useState<CatalogResponse | null>(initialPopular);
  const [featuredUnavailable, setFeaturedUnavailable] = useState(false);
  const discovery = !q && !genre && page === 1;
  // The default discovery tab is the release calendar, which loads its own schedule.
  const needsData = !(discovery && tab !== "popular");

  useEffect(() => {
    if (typeof window !== "undefined") {
      const search = new URLSearchParams(window.location.search);
      setQ(search.get("q") ?? initialQuery ?? "");
      setGenre(search.get("genre") ?? initialGenre ?? "");
      setTab(search.get("tab") ?? initialTab ?? "trending");
      setPage(Math.max(1, Number(search.get("page")) || initialPage || 1));
    }
  }, [initialQuery, initialGenre, initialTab, initialPage]);

  function update(values: Record<string, string>, scroll = true) {
    const current = typeof window !== "undefined" ? new URLSearchParams(window.location.search) : new URLSearchParams();
    Object.entries(values).forEach(([k, v]) => v ? current.set(k, v) : current.delete(k));
    router.push(`/?${current.toString()}`, { scroll });
  }

  useEffect(() => {
    if (!needsData) return;
    let active = true;
    setLoading(true);
    setError("");
    const request = q || genre ? searchCatalog(q, genre, page, 24) : tab === "popular" ? getCatalogPopular(page, 24) : getCatalogSeasonal(page, 24);
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
  }, [q, genre, tab, page, retry, needsData]);

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

  const featured = discovery ? (popular?.items || []).filter(anime =>
    anime.status !== "NOT_YET_RELEASED" && (anime.banner_image || anime.media_poster_image || anime.poster_image)) : [];
  const heroLoading = discovery && !popular && !featuredUnavailable;
  const cards = data?.items?.map(anime => <AnimeCatalogCard key={anime.media_id || anime.id} anime={anime} seasonal={!q && !genre && tab !== "popular"} />);

  return <main className={`catalog-page ${discovery && !error && (featured.length || heroLoading) ? "has-feature" : ""}`}>
    {featured.length > 0 && <FeaturedAnimeCarousel items={featured} />}
    {heroLoading && !error && <div className="hero-skeleton" role="status" aria-label={t("Chargement du catalogue")}><div /><div /></div>}
    {discovery && !error && <section className="season-discovery" aria-label={t("Catalogue")}>
      <div className="catalog-tabs-row page-inset">
        <div className="catalog-tabs" role="group" aria-label={t("Catalogue")}>
          <button type="button" aria-pressed={tab !== "popular"} onClick={() => update({ tab: "", page: "1" }, false)}>{t("Calendrier")}</button>
          <button type="button" aria-pressed={tab === "popular"} onClick={() => update({ tab: "popular", page: "1" }, false)}>{t("Les incontournables")}</button>
        </div>
      </div>
      {tab === "popular" ? (data && data.items && data.items.length > 0 && <>
        <div className="section-heading"><div className="section-title"><span className="eyebrow">{t("Catalogue")}</span><h2 id="season-heading" className="serif">{t("Les incontournables")}</h2></div></div>
        <SeasonalGrid count={data.items.length}>{cards}</SeasonalGrid>
      </>) : <ReleaseCalendar />}
    </section>}
    {discovery && !error && <AccountCta />}
    {(!discovery || error || (loading && needsData) || (needsData && data && data.items && data.items.length === 0)) && <div className="catalog-tools page-inset">
      {error ? <ErrorAlert className="my-8" message={error} code={errorCodeValue} onRetry={() => setRetry(retry + 1)} /> : (loading && needsData) ? <p role="status" className="catalog-message">{t("Chargement…")}</p> : <>
        {!discovery && <section><h1 className="results-heading">{q ? t("Résultats pour « {query} »", {query:q}) : genre || t("Explorer les animes")}</h1><div className="poster-grid">{cards}</div></section>}
        {data && data.items && data.items.length === 0 && <p className="catalog-message">{t("Aucun anime trouvé.")}</p>}
      </>}
      {!discovery && data && <nav aria-label={t("Pagination")} className="catalog-pagination"><button disabled={page <= 1} onClick={() => update({page:String(page - 1)})}><ArrowLeft size={16} /> {" "}{t("Précédent")}</button><span>{t("Page")}{" "}{page}</span><button disabled={!data?.has_next_page} onClick={() => update({page:String(page + 1)})}>{t("Suivant")}{" "}<ArrowRight size={16} /></button></nav>}
    </div>}
  </main>;
}
