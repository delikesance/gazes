"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown, Lock, Plus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { AuthError } from "@/lib/auth";
import { loadLists, setInList, useLists } from "@/lib/lists";
import { addToWatchlist, removeFromWatchlist, useInWatchlist } from "@/lib/watchlist";
import { useAuth } from "./AuthProvider";
import { CollectionNameDialog } from "./CollectionDialogs";

/** The single "Ajouter à…" control of an anime page: "À voir plus tard" for everyone, plus the collections once signed in. */
export function AddToMenu({ animeId }: { animeId: number }) {
  const { t } = useI18n();
  const { user } = useAuth();
  const lists = useLists();
  const saved = useInWatchlist(animeId);
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => { if (user && open && lists === null) void loadLists(); }, [user, open, lists]);
  useEffect(() => {
    if (!open) return;
    const away = (event: PointerEvent) => { if (!creating && !root.current?.contains(event.target as Node)) setOpen(false); };
    const esc = (event: KeyboardEvent) => { if (event.key === "Escape" && !creating) setOpen(false); };
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", esc);
    return () => { document.removeEventListener("pointerdown", away); document.removeEventListener("keydown", esc); };
  }, [open, creating]);

  const inCollection = (lists ?? []).some((list) => list.anime_ids.includes(animeId));
  const toggleList = (id: number, inList: boolean) => {
    setError(null);
    setInList(id, animeId, !inList).catch((err) => setError(t(err instanceof AuthError && (err.code === "too_many_lists" || err.code === "list_full") ? "Limite atteinte pour cette liste." : "Action impossible pour le moment.")));
  };

  return (
    <div className="anime-note" ref={root}>
      <button type="button" className="clay clay-secondary watchlist-button" aria-expanded={open} aria-haspopup="true" onClick={() => setOpen(!open)}>
        {saved || inCollection ? <Check size={16} aria-hidden="true" /> : <Plus size={16} aria-hidden="true" />}
        {t("Ajouter à…")}<ChevronDown size={16} aria-hidden="true" />
      </button>
      {open && (
        <div className="anime-note-panel add-to-panel" role="group" aria-label={t("Ajouter à")}>
          <span className="eyebrow">{t("Ajouter à")}</span>
          <label className="add-to-row"><input type="checkbox" checked={saved} onChange={() => (saved ? removeFromWatchlist(animeId) : addToWatchlist(animeId))} />{t("À voir plus tard")}</label>
          <hr className="add-to-sep" />
          <span className="eyebrow">{!user && <Lock size={12} aria-hidden="true" />}{t("Collections")}</span>
          {!user && <>
            <p className="history-empty">{t("Connectez-vous pour ranger vos animes dans des listes nommées.")}</p>
            <div><Link href="/login" className="clay clay-secondary clay-sm">{t("Se connecter")}</Link></div>
          </>}
          {user && <>
            {lists === null && <p className="history-empty">…</p>}
            {(lists ?? []).map((list) => {
              const inList = list.anime_ids.includes(animeId);
              return <label key={list.id} className="add-to-row"><input type="checkbox" checked={inList} onChange={() => toggleList(list.id, inList)} /><span className="truncate">{list.name}</span><span className="add-to-count">{list.anime_ids.length}</span></label>;
            })}
            <button type="button" className="add-to-new" onClick={() => setCreating(true)}><Plus size={16} aria-hidden="true" />{t("Nouvelle collection")}</button>
          </>}
          {error && <p className="field-error" role="alert">{error}</p>}
        </div>
      )}
      {creating && <CollectionNameDialog lists={lists ?? []} onClose={() => setCreating(false)} />}
    </div>
  );
}
