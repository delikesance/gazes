"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { ChevronDown, ChevronUp, Pencil, Plus, Trash2, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getFranchise } from "@/lib/api";
import { loadLists, setInList, useLists } from "@/lib/lists";
import type { UserList } from "@/lib/auth";
import { CollectionNameDialog, DeleteCollectionDialog } from "./CollectionDialogs";
import { LazyImage } from "./ui/LazyImage";

type Info = { title: string; poster?: string };
type Dialog = { kind: "create" } | { kind: "rename"; list: UserList } | { kind: "delete"; list: UserList };
const infoCache = new Map<number, Info>();

/** The viewer's collections (named lists), each as a poster grid with rename, delete and per-anime removal. */
export function NamedLists() {
  const { t } = useI18n();
  const lists = useLists();
  const [, bump] = useState(0);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [collapsed, setCollapsed] = useState<number[]>([]);
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
  const toggle = (id: number) => setCollapsed((current) => current.includes(id) ? current.filter((x) => x !== id) : [...current, id]);
  const createButton = <button type="button" className="clay clay-primary clay-sm" onClick={() => setDialog({ kind: "create" })}><Plus size={16} aria-hidden="true" />{t("Nouvelle collection")}</button>;

  return <>
    {lists.length === 0 ? (
      <div className="library-empty">
        <span className="eyebrow">{t("Aucune collection")}</span>
        <h2 className="serif">{t("Créez votre première collection.")}</h2>
        <p>{t("Donnez-lui un nom, puis ajoutez des animes depuis leur fiche avec « Ajouter à… ».")}</p>
        <div>{createButton}</div>
      </div>
    ) : <>
      <div className="library-actions">
        <span className="eyebrow">{t("{count} collections", { count: lists.length })}</span>
        {createButton}
      </div>
      {lists.map((list) => {
        const open = !collapsed.includes(list.id);
        return (
          <section key={list.id} className="continue-watching" aria-label={list.name}>
            <div className="section-heading">
              <div className="section-title"><h2 className="serif">{list.name}</h2><span className="eyebrow">{t("{count} animes", { count: list.anime_ids.length })}</span></div>
              <span className="library-icons">
                <button type="button" className="clay clay-secondary clay-sm" aria-label={t("Renommer {name}", { name: list.name })} onClick={() => setDialog({ kind: "rename", list })}><Pencil size={14} aria-hidden="true" /></button>
                <button type="button" className="clay clay-secondary clay-sm" aria-label={t("Supprimer {name}", { name: list.name })} onClick={() => setDialog({ kind: "delete", list })}><Trash2 size={14} aria-hidden="true" /></button>
                <button type="button" className="clay clay-secondary clay-sm" aria-expanded={open} aria-label={t(open ? "Replier {name}" : "Déplier {name}", { name: list.name })} onClick={() => toggle(list.id)}>{open ? <ChevronUp size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}</button>
              </span>
            </div>
            {open && list.anime_ids.length === 0 && <p className="history-empty">{t("Collection vide. Ajoutez des animes depuis leur fiche avec « Ajouter à… ».")}</p>}
            {open && list.anime_ids.length > 0 && <ul className="poster-grid page-inset watchlist-grid">
              {list.anime_ids.slice(0, 24).map((id) => {
                const info = infoCache.get(id);
                const title = info?.title || t("Anime");
                return <li key={id} className="poster-cell">
                  <Link href={`/anime/${id}`} className="poster-card" title={title}>
                    <LazyImage src={info?.poster} alt={title} aspectRatio="" className="poster-art" />
                    <div className="poster-overlay"><div><h3>{title}</h3></div></div>
                  </Link>
                  <button type="button" className="poster-hide" onClick={() => void setInList(list.id, id, false)} aria-label={t("Retirer {title} de la collection", { title })} title={t("Retirer de la collection")}><X size={16} aria-hidden="true" /></button>
                </li>;
              })}
            </ul>}
          </section>
        );
      })}
    </>}
    {dialog?.kind === "create" && <CollectionNameDialog lists={lists} onClose={() => setDialog(null)} />}
    {dialog?.kind === "rename" && <CollectionNameDialog list={dialog.list} lists={lists} onClose={() => setDialog(null)} />}
    {dialog?.kind === "delete" && <DeleteCollectionDialog list={dialog.list} onClose={() => setDialog(null)} />}
  </>;
}
