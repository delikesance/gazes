"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { getCatalogForYou } from "@/lib/api";
import { hideAnime, listHidden } from "@/lib/hidden-anime";
import { errorCode } from "@/lib/error-code";
import { recentSessions, watchedSeeds } from "@/components/CatalogBrowser";
import { AnimeCatalogCard } from "@/components/AnimeCatalogCard";
import { SeasonalGrid } from "@/components/SeasonalGrid";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageGrid } from "@/components/ui/PageGrid";
import { Scribble } from "@/components/ui/Scribble";
import type { CatalogResponse } from "@/types/api";

type Anime = NonNullable<CatalogResponse["items"]>[number];
const PAGE_SIZE = 28;

/** Endless feed of suggestions built from this device's (or the account's) watch history. */
export default function ForYouPage() {
  const { t } = useI18n();
  const [items, setItems] = useState<Anime[]>([]);
  const [page, setPage] = useState(1);
  const [hasNext, setHasNext] = useState(true);
  const [loadedPage, setLoadedPage] = useState(0);
  const [error, setError] = useState("");
  const [code, setCode] = useState("");
  const [retry, setRetry] = useState(0);
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    getCatalogForYou(watchedSeeds(), recentSessions(), listHidden(), page, PAGE_SIZE).then((result) => {
      if (!active) return;
      setItems((current) => {
        const seen = new Set(current.map((anime) => anime.id));
        return [...current, ...(result.items ?? []).filter((anime) => !seen.has(anime.id))];
      });
      setHasNext(Boolean(result.has_next_page) && (result.items?.length ?? 0) > 0);
      setLoadedPage(page);
    }).catch((err) => {
      if (!active) return;
      setError(err instanceof Error ? err.message : "Impossible de charger le catalogue.");
      setCode(errorCode(err, "CAT"));
      setLoadedPage(page);
    });
    return () => { active = false; };
  }, [page, retry]);

  const loading = !error && loadedPage < page;
  const more = useCallback(() => setPage((p) => p + 1), []);
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasNext || loading || error) return;
    const observer = new IntersectionObserver((entries) => { if (entries[0].isIntersecting) more(); }, { rootMargin: "800px 0px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasNext, loading, error, more, items.length]);

  function hide(anime: { id: number; media_id?: number }) {
    hideAnime(anime.id);
    if (anime.media_id) hideAnime(anime.media_id);
    setItems((current) => current.filter((item) => item.id !== anime.id));
  }

  return (
    <main className="history-page">
      <PageGrid />
      <Scribble shape="b" width={380} rotate={-6} style={{ right: 40, top: 120 }} />
      <div className="history-inner page-inset">
        <span className="eyebrow">{t("Suggestions")}</span>
        <h1 className="serif">{t("Pour vous")}</h1>
      </div>
      {items.length > 0 && <SeasonalGrid count={items.length} evenRows={false}>
        {items.map((anime) => <AnimeCatalogCard key={anime.media_id || anime.id} anime={anime} seasonal={false} onHide={hide} />)}
      </SeasonalGrid>}
      <div className="page-inset">
        {error ? <ErrorAlert className="my-8" message={error} code={code} onRetry={() => { setError(""); setRetry((n) => n + 1); }} />
          : loading ? <p role="status" className="catalog-message">{t("Chargement…")}</p>
          : !hasNext && items.length === 0 ? <p className="catalog-message">{t("Aucun anime trouvé.")}</p> : null}
      </div>
      <div ref={sentinel} aria-hidden="true" />
    </main>
  );
}
