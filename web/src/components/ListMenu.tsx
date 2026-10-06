"use client";
import { useEffect, useState, type FormEvent } from "react";
import { Check, ListPlus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { AuthError } from "@/lib/auth";
import { createNamedList, loadLists, MAX_LIST_NAME, setInList, useLists } from "@/lib/lists";
import { useAuth } from "./AuthProvider";

/** "Mes listes" on an anime page: tick the named lists it belongs to, or start a new one. Signed-in only. */
export function ListMenu({ animeId }: { animeId: number }) {
  const { t } = useI18n();
  const { user } = useAuth();
  const lists = useLists();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => { if (user && open && lists === null) void loadLists(); }, [user, open, lists]);
  if (!user) return null;

  const fail = (err: unknown) => setError(t(err instanceof AuthError && (err.code === "too_many_lists" || err.code === "list_full") ? "Limite atteinte pour cette liste." : "Action impossible pour le moment."));
  const toggle = (id: number, inList: boolean) => { setError(null); setInList(id, animeId, !inList).catch(fail); };
  const create = (event: FormEvent) => {
    event.preventDefault();
    const clean = name.trim();
    if (!clean) return;
    setError(null);
    createNamedList(clean).then(() => setName("")).catch(fail);
  };

  return (
    <div className="anime-note">
      <button type="button" className="clay clay-secondary watchlist-button" aria-expanded={open} onClick={() => setOpen(!open)}>
        <ListPlus size={16} aria-hidden="true" />{t("Mes listes")}
      </button>
      {open && (
        <div className="anime-note-panel" role="group" aria-label={t("Mes listes")}>
          {(lists ?? []).length === 0 && lists !== null && <p className="history-empty">{t("Aucune liste pour l’instant.")}</p>}
          {(lists ?? []).map((list) => {
            const inList = list.anime_ids.includes(animeId);
            return (
              <button key={list.id} type="button" className="list-row" aria-pressed={inList} onClick={() => toggle(list.id, inList)}>
                <span className="truncate">{list.name}</span>{inList && <Check size={16} aria-hidden="true" />}
              </button>
            );
          })}
          <form onSubmit={create} className="account-delete">
            <label htmlFor={`new-list-${animeId}`}>{t("Nouvelle liste")}</label>
            <div className="field-pill"><input id={`new-list-${animeId}`} value={name} onChange={(e) => setName(e.target.value)} maxLength={MAX_LIST_NAME} /></div>
            <div><button type="submit" className="clay clay-primary clay-sm" disabled={!name.trim()}>{t("Créer")}</button></div>
          </form>
          {error && <p className="field-error" role="alert">{error}</p>}
        </div>
      )}
    </div>
  );
}
