"use client";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { Clock, Search, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { searchCatalog } from "@/lib/api";
import { genreLabel, parseList } from "@/lib/genres";
import { errorCode } from "@/lib/error-code";
import { rememberResults } from "@/lib/navigation-history";
import { readRecentSearches, saveRecentSearch } from "@/lib/recent-searches";
import { setUrlParams, useUrlParam } from "@/lib/url-state";
import { AnimeCatalogCard } from "./AnimeCatalogCard";
import { SeasonalGrid } from "./SeasonalGrid";
import { ErrorAlert } from "./ErrorAlert";
import { GenreFilter } from "./GenreFilter";
import { PageGrid } from "./ui/PageGrid";
import { Scribble } from "./ui/Scribble";
import type { CatalogResponse } from "@/types/api";

type Anime = NonNullable<CatalogResponse["items"]>[number];
type Results = { key: string; items: Anime[]; page: number; hasNext: boolean; error: string; code: string };
const PAGE_SIZE = 28;
const noop = () => () => {};

/** Every anime, most popular first, narrowed by title and by genres kept in the address. Pages load as the viewer scrolls. */
export function SearchBrowser() {
  const { t } = useI18n();
  const q = (useUrlParam("q") ?? "").trim();
  const genresParam = useUrlParam("genres") ?? "";
  const excludeParam = useUrlParam("exclude") ?? "";
  const include = parseList(genresParam);
  const exclude = parseList(excludeParam);
  const key = `${q}|${genresParam}|${excludeParam}`;
  const filtered = Boolean(q || include.length || exclude.length);

  const [results, setResults] = useState<Results | null>(null);
  const [retry, setRetry] = useState(0);
  const [hasText, setHasText] = useState(false);
  const hydrated = useSyncExternalStore(noop, () => true, () => false);
  const inputRef = useRef<HTMLInputElement>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  const typing = useRef<ReturnType<typeof setTimeout>>(undefined);
  const loadingMore = useRef(false);

  // First page of the current search; a newer search cancels the one still running.
  useEffect(() => {
    const controller = new AbortController();
    searchCatalog(q, genresParam, 1, PAGE_SIZE, excludeParam, controller.signal).then((result) => {
      setResults({ key, items: result.items ?? [], page: 1, hasNext: Boolean(result.has_next_page), error: "", code: "" });
    }, (error) => {
      if (error?.name === "AbortError") return;
      setResults({ key, items: [], page: 1, hasNext: false, error: error instanceof Error ? error.message : "Impossible de charger le catalogue.", code: errorCode(error, "CAT") });
    });
    return () => controller.abort();
  }, [key, q, genresParam, excludeParam, retry]);

  // The field follows the address (back button, cleared filters) but never while the viewer is typing in it.
  useEffect(() => {
    const el = inputRef.current;
    if (el && document.activeElement !== el) { el.value = q; }
  }, [q]);
  useEffect(() => {
    if (window.matchMedia("(pointer: fine)").matches) inputRef.current?.focus();
  }, []);
  // A series page opened from here can offer the way back.
  useEffect(() => {
    rememberResults(`${window.location.pathname}${window.location.search}`, q && q.length <= 20 ? t("Résultats pour « {query} »", { query: q }) : t("Résultats"));
  }, [key, q, t]);

  const current = results?.key === key ? results : null;
  const loading = !current;
  const shown = current ?? results;

  const loadMore = () => {
    if (!current || !current.hasNext || loadingMore.current) return;
    loadingMore.current = true;
    searchCatalog(q, genresParam, current.page + 1, PAGE_SIZE, excludeParam).then((result) => {
      setResults((prev) => {
        if (!prev || prev.key !== key) return prev;
        const seen = new Set(prev.items.map((anime) => anime.id));
        return { ...prev, items: [...prev.items, ...(result.items ?? []).filter((anime) => !seen.has(anime.id))], page: prev.page + 1, hasNext: Boolean(result.has_next_page) && (result.items?.length ?? 0) > 0 };
      });
    }).catch(() => setResults((prev) => (prev && prev.key === key ? { ...prev, hasNext: false } : prev))).finally(() => { loadingMore.current = false; });
  };
  const loadMoreRef = useRef(loadMore);
  useEffect(() => { loadMoreRef.current = loadMore; });

  useEffect(() => {
    const node = sentinel.current;
    if (!node || !current?.hasNext) return;
    const observer = new IntersectionObserver((entries) => { if (entries[0].isIntersecting) loadMoreRef.current(); }, { rootMargin: "800px 0px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [current?.hasNext, current?.items.length]);

  const apply = (text: string) => {
    const value = text.trim();
    if (value.length >= 3) saveRecentSearch(value);
    setUrlParams({ q: value || null });
  };
  const onType = (value: string) => {
    setHasText(value !== "");
    if (typing.current) clearTimeout(typing.current);
    typing.current = setTimeout(() => apply(value), 350);
  };
  const clearAll = () => {
    if (typing.current) clearTimeout(typing.current);
    if (inputRef.current) inputRef.current.value = "";
    setHasText(false);
    setUrlParams({ q: null, genres: null, exclude: null });
  };
  const recents = hydrated && !hasText && !q ? readRecentSearches() : [];
  const summary = [include.map(genreLabel).join(", "), exclude.length ? t("sans {genres}", { genres: exclude.map(genreLabel).join(", ") }) : ""].filter(Boolean).join(" · ");

  return (
    <main className="catalog-page search-page">
      <PageGrid />
      <Scribble shape="b" width={380} rotate={-6} style={{ right: 40, top: 120 }} />
      <div className="search-inner page-inset">
        <span className="eyebrow">{t("Recherche")}</span>
        <h1 className="serif">{filtered ? (q ? t("Résultats pour « {query} »", { query: q }) : summary) : t("Tous les animes")}</h1>
        <form className="search-field" role="search" onSubmit={(event) => { event.preventDefault(); if (typing.current) clearTimeout(typing.current); apply(inputRef.current?.value ?? ""); inputRef.current?.blur(); }}>
          <Search size={20} aria-hidden="true" />
          <input ref={inputRef} name="q" type="search" autoComplete="off" enterKeyHint="search" aria-label={t("Rechercher un anime")} placeholder={t("Rechercher un anime…")} defaultValue="" onChange={(event) => onType(event.target.value)} />
          {(hasText || filtered) && <button type="button" aria-label={t("Effacer la recherche")} onClick={clearAll}><X size={18} aria-hidden="true" /></button>}
        </form>
        {recents.length > 0 && <div className="search-recent-chips" role="group" aria-label={t("Recherches récentes")}>
          {recents.map((text) => <button key={text} type="button" onClick={() => { if (inputRef.current) inputRef.current.value = text; setHasText(true); apply(text); }}><Clock size={14} aria-hidden="true" />{text}</button>)}
        </div>}
        <p className="search-hint">{t("Clic gauche : inclure · Clic droit ou appui long : exclure")}</p>
        <GenreFilter include={include} exclude={exclude} onChange={(inc, exc) => setUrlParams({ genres: inc.join(",") || null, exclude: exc.join(",") || null })} />
      </div>
      {shown && shown.items.length > 0 && <div data-stale={!current} className="search-results"><SeasonalGrid count={shown.items.length} evenRows={false} keepColumns>
        {shown.items.map((anime) => <AnimeCatalogCard key={anime.media_id || anime.id} anime={anime} seasonal={false} />)}
      </SeasonalGrid></div>}
      <div className="page-inset">
        {shown?.error && current ? <ErrorAlert className="my-8" message={shown.error} code={shown.code} onRetry={() => { setResults(null); setRetry((n) => n + 1); }} />
          : loading ? <p role="status" className="catalog-message">{t("Chargement…")}</p>
          : current && current.items.length === 0 ? <div className="catalog-message"><p>{t("Aucun anime ne correspond à cette recherche.")}</p>{filtered && <button type="button" className="clay clay-secondary clay-sm" onClick={clearAll}>{t("Effacer les filtres")}</button>}</div> : null}
      </div>
      <div ref={sentinel} aria-hidden="true" />
    </main>
  );
}
