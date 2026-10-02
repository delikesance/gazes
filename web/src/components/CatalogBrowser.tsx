"use client";
import { useI18n } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { AnimeCatalogCard } from "./AnimeCatalogCard";
import { FeaturedAnimeCarousel } from "./FeaturedAnimeCarousel";
import { SeasonalGrid } from "./SeasonalGrid";
import { AccountCta } from "./AccountCta";
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
  const [retry, setRetry] = useState(0);
  const [popular, setPopular] = useState<CatalogResponse | null>(initialPopular);
  const [featuredUnavailable, setFeaturedUnavailable] = useState(false);
  const discovery = !q && !genre && page === 1;

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
        setLoading(false);
      }
    });
    return () => {
      active = false;
    };
  }, [q, genre, tab, page, retry]);

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
  const seasonLabel = data?.season ? ({WINTER:"Hiver", SPRING:"Printemps", SUMMER:"Été", FALL:"Automne"} as Record<string,string>)[data.season] : "";
  const cards = data?.items?.map(anime => <AnimeCatalogCard key={anime.media_id || anime.id} anime={anime} seasonal={!q && !genre && tab !== "popular"} />);

  return <main className={`catalog-page ${discovery && !error && (featured.length || heroLoading) ? "has-feature" : ""}`}>
    {featured.length > 0 && <FeaturedAnimeCarousel items={featured} />}
    {heroLoading && !error && <div className="hero-skeleton" role="status" aria-label={t("Chargement du catalogue")}><div /><div /></div>}
    {discovery && data && data.items && data.items.length > 0 && <section className="season-discovery" aria-labelledby="season-heading">
      <div className="section-heading section-heading--tabs">
        <div className="section-title">
          <span className="eyebrow">{t("Catalogue")}</span>
          <h2 id="season-heading" className="serif">{tab === "popular" ? t("Les incontournables") : t("Cette saison")}</h2>
          {tab !== "popular" && seasonLabel && <span className="season-period">{t(seasonLabel)} {data?.season_year}</span>}
        </div>
        <div className="catalog-tabs" role="group" aria-label={t("Catalogue")}>
          <button type="button" aria-pressed={tab !== "popular"} onClick={() => update({ tab: "", page: "1" }, false)}>{t("Cette saison")}</button>
          <button type="button" aria-pressed={tab === "popular"} onClick={() => update({ tab: "popular", page: "1" }, false)}>{t("Les incontournables")}</button>
        </div>
      </div>
      <SeasonalGrid count={data.items.length}>{cards}</SeasonalGrid>
    </section>}
    {discovery && !error && data && data.items && data.items.length > 0 && <AccountCta />}
    {(!discovery || error || loading || (data && data.items && data.items.length === 0)) && <div className="catalog-tools page-inset">
      {error ? <div role="alert" className="catalog-message">{t(error)} <button className="text-action" onClick={() => setRetry(retry + 1)}>{t("Réessayer")}</button></div> : loading ? <p role="status" className="catalog-message">{t("Chargement…")}</p> : <>
        {!discovery && <section><h1 className="results-heading">{q ? t("Résultats pour « {query} »", {query:q}) : genre || t("Explorer les animes")}</h1><div className="poster-grid">{cards}</div></section>}
        {data && data.items && data.items.length === 0 && <p className="catalog-message">{t("Aucun anime trouvé.")}</p>}
      </>}
      {!discovery && data && <nav aria-label={t("Pagination")} className="catalog-pagination"><button disabled={page <= 1} onClick={() => update({page:String(page - 1)})}><ArrowLeft size={16} /> {" "}{t("Précédent")}</button><span>{t("Page")}{" "}{page}</span><button disabled={!data?.has_next_page} onClick={() => update({page:String(page + 1)})}>{t("Suivant")}{" "}<ArrowRight size={16} /></button></nav>}
    </div>}
  </main>;
}
