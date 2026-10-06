"use client";
/* eslint-disable @next/next/no-img-element -- poster URLs come from the catalog CDN */
import Link from "next/link";
import { useEffect, useState } from "react";
import { Lock, Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listProgress, type SavedProgress } from "@/lib/watch-progress";
import { getSeason } from "@/lib/api";
import { cachedImage } from "@/lib/image";
import { PageGrid } from "@/components/ui/PageGrid";
import { Scribble } from "@/components/ui/Scribble";
import { WatchlistShelf } from "@/components/WatchlistShelf";
import { AniListImport } from "@/components/AniListImport";
import { NamedLists } from "@/components/NamedLists";
import { useAuth } from "@/components/AuthProvider";
import { setUrlParams, useUrlParam } from "@/lib/url-state";
import { useWatchlist } from "@/lib/watchlist";
import { loadLists, useLists } from "@/lib/lists";

type Info = { title: string; poster?: string };
const infoCache = new Map<number, Info>();

export default function HistoryPage() {
  const { t, locale } = useI18n();
  const tabParam = useUrlParam("tab");
  // "listes" is the former name of the collections tab: old links keep working.
  const tab = tabParam === "liste" ? "liste" : tabParam === "collections" || tabParam === "listes" ? "collections" : "reprendre";
  const { user } = useAuth();
  const watchlist = useWatchlist();
  const lists = useLists();
  const [items, setItems] = useState<SavedProgress[] | null>(null);
  const [info, setInfo] = useState<Record<number, Info>>({});

  useEffect(() => { if (user) void loadLists(); }, [user]);

  useEffect(() => {
    const refresh = () => setItems(listProgress());
    refresh();
    window.addEventListener("gazes-progress-change", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("gazes-progress-change", refresh); window.removeEventListener("storage", refresh); };
  }, []);

  // Entries saved before titles were stored (or from another device) are completed from the catalog.
  useEffect(() => {
    if (!items) return;
    const controller = new AbortController();
    for (const item of items) {
      if (infoCache.has(item.season)) continue;
      getSeason(item.animeId || item.season, item.season, controller.signal).then((season) => {
        const value = { title: season.display_title, poster: season.poster_image };
        infoCache.set(item.season, value);
        setInfo((current) => ({ ...current, [item.season]: value }));
      }).catch(() => {});
    }
    return () => controller.abort();
  }, [items]);

  return (
    <main className="history-page">
      <PageGrid />
      <Scribble shape="b" width={380} rotate={-6} style={{ right: 40, top: 120 }} />
      <div className="history-inner page-inset">
        <span className="eyebrow">{t("Vos animes")}</span>
        <h1 className="serif">{t("Bibliothèque")}</h1>
        <div className="catalog-tabs library-tabs" role="group" aria-label={t("Bibliothèque")}>
          <button type="button" aria-pressed={tab === "reprendre"} onClick={() => setUrlParams({ tab: null })}>{t("Reprendre")}{items && <span className="tab-count">{items.length}</span>}</button>
          <button type="button" aria-pressed={tab === "liste"} onClick={() => setUrlParams({ tab: "liste" })}>{t("À voir plus tard")}<span className="tab-count">{watchlist.length}</span></button>
          <button type="button" aria-pressed={tab === "collections"} onClick={() => setUrlParams({ tab: "collections" })}>{!user && <Lock size={13} aria-hidden="true" />}{t("Collections")}{user && lists && <span className="tab-count">{lists.length}</span>}</button>
        </div>
        <p className="library-help">{t(tab === "liste" ? "La file d’attente. Une seule liste, sans compte nécessaire." : tab === "collections" ? (user ? "Des listes nommées, disponibles sur tous vos appareils." : "Des listes nommées. Un compte est nécessaire.") : "Les épisodes commencés. Cliquez sur une affiche pour reprendre là où vous vous êtes arrêté.")}</p>
        {tab === "liste" && (watchlist.length === 0
          ? <div className="library-empty">
              <span className="eyebrow">{t("Liste vide")}</span>
              <h2 className="serif">{t("Gardez de côté ce que vous voulez voir.")}</h2>
              <p>{t("Ajoutez un anime depuis sa fiche ou depuis le calendrier. Une liste existe déjà ailleurs ? Importez-la.")}</p>
              <div className="library-buttons"><Link href="/" className="clay clay-primary clay-sm">{t("Parcourir le catalogue")}</Link><AniListImport /></div>
            </div>
          : <WatchlistShelf embedded><AniListImport /></WatchlistShelf>)}
        {tab === "collections" && (user
          ? <NamedLists />
          : <div className="library-empty">
              <span className="eyebrow">{t("Compte requis")}</span>
              <h2 className="serif">{t("Rangez vos animes dans des collections.")}</h2>
              <p>{t("« Classiques », « À revoir en famille », « Pour le week-end » : créez autant de listes que vous voulez et retrouvez-les sur tous vos appareils. La liste À voir plus tard reste intacte.")}</p>
              <div className="library-buttons"><Link href="/login" className="clay clay-primary clay-sm">{t("Se connecter")}</Link><Link href="/register" className="clay clay-secondary clay-sm">{t("Créer un compte")}</Link></div>
            </div>)}
        {tab === "reprendre" && items && items.length === 0 && <div className="library-empty">
          <span className="eyebrow">{t("Rien à reprendre")}</span>
          <h2 className="serif">{t("Lancez un épisode, il apparaît ici.")}</h2>
          <p>{t("La progression est enregistrée automatiquement. Aucun compte n’est nécessaire.")}</p>
          <div><Link href="/" className="clay clay-primary clay-sm">{t("Parcourir le catalogue")}</Link></div>
        </div>}
        {tab === "reprendre" && <ul className="history-grid">
          {(items || []).map((item) => {
            const meta = info[item.season] ?? infoCache.get(item.season);
            const title = item.title || meta?.title || t("Anime");
            // Watched share of the episode; unknown (other device, older entry) shows the poster in full colour.
            const watched = item.duration ? Math.min(100, Math.max(0, Math.round((item.position / item.duration) * 100))) : 100;
            return (
              <li key={item.season}>
                <Link href={`/anime/${item.animeId || item.season}/seasons/${item.season}/episodes/${item.episode}`} className="history-card">
                  <span className="history-poster" role="img" aria-label={`${title}, ${watched} %`}>
                    {meta?.poster && <img src={cachedImage(meta.poster)} alt="" loading="lazy" />}
                    {meta?.poster && watched < 100 && <img className="history-poster-gray" src={cachedImage(meta.poster)} alt="" aria-hidden="true" loading="lazy" style={{ clipPath: `inset(0 0 ${watched}% 0)` }} />}
                    <span className="history-play"><Play size={18} fill="currentColor" /></span>
                  </span>
                  <span className="history-meta">
                    <span className="history-episode">{t("Épisode")} {item.episode}</span>
                    {item.updatedAt > 1e9 && <span className="history-date">{new Date(item.updatedAt * 1000).toLocaleDateString(locale)}</span>}
                  </span>
                  <span className="history-title">{title}</span>
                </Link>
              </li>
            );
          })}
        </ul>}
      </div>
    </main>
  );
}
