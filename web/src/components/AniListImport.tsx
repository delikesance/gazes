"use client";
import { useRef, useState, type FormEvent } from "react";
import { Download, Loader2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getFranchise } from "@/lib/api";
import { addManyToWatchlist } from "@/lib/watchlist";
import { fetchAniListList, ImportError, resolveFranchises } from "@/lib/anilist-import";

type Result = { added: number; skipped: number; truncated: boolean; stopped: boolean };

/** "Ma liste" import from a public AniList list (watching, planning, paused). */
export function AniListImport() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [user, setUser] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Result | null>(null);
  const controller = useRef<AbortController | null>(null);

  const run = async (event: FormEvent) => {
    event.preventDefault();
    if (!user.trim() || busy) return;
    controller.current = new AbortController();
    const { signal } = controller.current;
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const list = await fetchAniListList(user, signal);
      setProgress({ done: 0, total: list.entries.length });
      const resolved = await resolveFranchises(list.entries, async (id) => (await getFranchise(id, signal)).id, (done) => setProgress({ done, total: list.entries.length }), signal);
      const added = addManyToWatchlist(resolved.franchiseIds);
      setResult({ added, skipped: resolved.skipped + (resolved.stopped ? list.entries.length - resolved.resolved - resolved.skipped : 0), truncated: list.truncated, stopped: resolved.stopped });
    } catch (err) {
      const code = err instanceof ImportError ? err.code : "network";
      setError(t({ user_not_found: "Utilisateur AniList introuvable.", list_private: "Cette liste AniList est privée.", busy: "AniList est très sollicité, réessayez dans quelques minutes.", network: "Impossible d’importer la liste pour le moment." }[code]));
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  if (!open) return <div><button type="button" className="clay clay-secondary clay-sm" onClick={() => setOpen(true)}><Download size={16} aria-hidden="true" />{t("Importer depuis AniList")}</button></div>;
  return (
    <form onSubmit={run} className="account-delete">
      <label htmlFor="anilist-user">{t("Pseudo AniList (liste publique)")}</label>
      <div className="field-pill"><input id="anilist-user" value={user} onChange={(e) => setUser(e.target.value)} maxLength={20} autoComplete="off" disabled={busy} /></div>
      <div className="account-delete-actions">
        <button type="submit" className="clay clay-primary clay-sm" disabled={busy || !user.trim()}>
          {busy ? <><Loader2 size={14} className="animate-spin" aria-hidden="true" />{progress ? `${progress.done} / ${progress.total}` : "…"}</> : t("Importer")}
        </button>
        <button type="button" className="clay clay-secondary clay-sm" onClick={() => { controller.current?.abort(); setOpen(false); setResult(null); setError(null); }}>{busy ? t("Annuler") : t("Fermer")}</button>
      </div>
      {error && <p className="field-error" role="alert">{error}</p>}
      {result && (
        <p role="status" className="catalog-message">
          {t("{count} anime ajoutés à votre liste.", { count: result.added })}
          {result.truncated && ` ${t("Seuls les 100 premiers titres ont été lus.")}`}
          {result.skipped > 0 && ` ${t(result.stopped ? "{count} titres n’ont pas pu être résolus (AniList est limité), relancez l’import plus tard." : "{count} titres n’ont pas pu être résolus.", { count: result.skipped })}`}
        </p>
      )}
    </form>
  );
}
