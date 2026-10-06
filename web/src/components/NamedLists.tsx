"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { Pencil, Trash2, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getFranchise } from "@/lib/api";
import { deleteNamedList, loadLists, renameNamedList, setInList, useLists } from "@/lib/lists";
import { LazyImage } from "./ui/LazyImage";

type Info = { title: string; poster?: string };
const infoCache = new Map<number, Info>();

/** The viewer's named lists, each as a poster grid with rename, delete and per-anime removal. */
export function NamedLists() {
  const { t } = useI18n();
  const lists = useLists();
  const [, bump] = useState(0);
  const key = (lists ?? []).flatMap((l) => l.anime_ids.slice(0, 24)).join(",");

  useEffect(() => { void loadLists(); }, []);
  useEffect(() => {
    const controller = new AbortController();
    for (const id of key ? key.split(",").map(Number) : []) {
      if (infoCache.has(id)) continue;
      getFranchise(id, controller.signal).then((f) => { infoCache.set(id, { title: f.title, poster: f.poster_image }); bump((n) => n + 1); }).catch(() => {});
    }
    return () => controller.abort();
  }, [key]);

  if (lists === null) return null;
  if (lists.length === 0) return <p className="history-empty">{t("Aucune liste pour l’instant. Créez-en une depuis la fiche d’un anime (bouton « Mes listes »).")}</p>;
  return <>
    {lists.map((list) => (
      <section key={list.id} className="continue-watching" aria-label={list.name}>
        <div className="section-heading">
          <div className="section-title"><h2 className="serif">{list.name}</h2></div>
          <span>
            <button type="button" className="clay clay-secondary clay-sm" aria-label={t("Renommer {name}", { name: list.name })} onClick={() => { const next = window.prompt(t("Nouveau nom"), list.name)?.trim(); if (next) void renameNamedList(list.id, next); }}><Pencil size={14} aria-hidden="true" /></button>{" "}
            <button type="button" className="clay clay-secondary clay-sm" aria-label={t("Supprimer {name}", { name: list.name })} onClick={() => { if (window.confirm(t("Supprimer la liste « {name} » ?", { name: list.name }))) void deleteNamedList(list.id); }}><Trash2 size={14} aria-hidden="true" /></button>
          </span>
        </div>
        {list.anime_ids.length === 0 && <p className="history-empty">{t("Cette liste est vide.")}</p>}
        <ul className="poster-grid page-inset watchlist-grid">
          {list.anime_ids.slice(0, 24).map((id) => {
            const info = infoCache.get(id);
            const title = info?.title || t("Anime");
            return <li key={id} className="poster-cell">
              <Link href={`/anime/${id}`} className="poster-card" title={title}>
                <LazyImage src={info?.poster} alt={title} aspectRatio="" className="poster-art" />
                <div className="poster-overlay"><div><h3>{title}</h3></div></div>
              </Link>
              <button type="button" className="poster-hide" onClick={() => void setInList(list.id, id, false)} aria-label={t("Retirer {title} de la liste", { title })} title={t("Retirer de la liste")}><X size={16} aria-hidden="true" /></button>
            </li>;
          })}
        </ul>
      </section>
    ))}
  </>;
}
