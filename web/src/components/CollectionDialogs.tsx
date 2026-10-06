"use client";
import { useState, type FormEvent } from "react";
import { TriangleAlert } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { AuthError, type UserList } from "@/lib/auth";
import { createNamedList, deleteNamedList, MAX_LIST_NAME, renameNamedList } from "@/lib/lists";
import { Dialog } from "./ui/Dialog";

/** Create (no `list`) or rename a collection; refuses an empty or already used name before calling the server. */
export function CollectionNameDialog({ list, lists, onClose }: { list?: UserList; lists: UserList[]; onClose: () => void }) {
  const { t } = useI18n();
  const [name, setName] = useState(list?.name ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const clean = name.trim();
  const taken = lists.some((other) => other.id !== list?.id && other.name.trim().toLowerCase() === clean.toLowerCase());

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!clean || busy) return;
    if (taken) { setError(t("Une collection « {name} » existe déjà.", { name: clean })); return; }
    setBusy(true);
    setError(null);
    try {
      if (list) await renameNamedList(list.id, clean); else await createNamedList(clean);
      onClose();
    } catch (err) {
      setError(t(err instanceof AuthError && err.code === "too_many_lists" ? "Limite atteinte pour cette liste." : "Action impossible pour le moment."));
      setBusy(false);
    }
  };

  return (
    <Dialog title={t(list ? "Renommer la collection" : "Nouvelle collection")} onClose={onClose}>
      <form onSubmit={submit} className="dialog-form">
        <label htmlFor="collection-name">{t(list ? "Nouveau nom" : "Nom")}</label>
        <div className="field-pill"><input id="collection-name" data-autofocus value={name} onChange={(e) => { setName(e.target.value); setError(null); }} maxLength={MAX_LIST_NAME} autoComplete="off" placeholder={list ? undefined : t("Ex. Classiques")} aria-invalid={error ? true : undefined} aria-describedby={error ? "collection-name-error" : undefined} /></div>
        {error && <p id="collection-name-error" className="field-error dialog-error" role="alert"><TriangleAlert size={14} aria-hidden="true" />{error}</p>}
        <div className="dialog-actions">
          <button type="button" className="clay clay-secondary" onClick={onClose}>{t("Annuler")}</button>
          <button type="submit" className="clay clay-primary" disabled={!clean || busy}>{t(list ? "Enregistrer" : "Créer")}</button>
        </div>
      </form>
    </Dialog>
  );
}

/** Confirmation before a collection is deleted; the animes themselves are untouched. */
export function DeleteCollectionDialog({ list, onClose }: { list: UserList; onClose: () => void }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const remove = async () => {
    setBusy(true);
    setError(false);
    try { await deleteNamedList(list.id); onClose(); } catch { setError(true); setBusy(false); }
  };
  return (
    <Dialog title={t("Supprimer « {name} » ?", { name: list.name })} onClose={onClose} role="alertdialog">
      <div className="dialog-form">
        <p className="dialog-text">{t("La collection disparaît. Les animes restent disponibles dans le catalogue, et dans À voir plus tard s’ils y figurent.")}</p>
        {error && <p className="field-error dialog-error" role="alert"><TriangleAlert size={14} aria-hidden="true" />{t("Action impossible pour le moment.")}</p>}
        <div className="dialog-actions">
          <button type="button" className="clay clay-secondary" data-autofocus onClick={onClose}>{t("Garder")}</button>
          <button type="button" className="clay clay-danger" disabled={busy} onClick={() => void remove()}>{t("Supprimer")}</button>
        </div>
      </div>
    </Dialog>
  );
}
