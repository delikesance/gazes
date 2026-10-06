"use client";
import { useRef, useState, type FormEvent } from "react";
import { Check, Download, Loader2, TriangleAlert } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getFranchise } from "@/lib/api";
import { addManyToWatchlist } from "@/lib/watchlist";
import { fetchAniListList, fetchMalTitles, ImportError, parseMalExport, resolveFranchises, type AniListList } from "@/lib/anilist-import";

type Result = { added: number; skipped: number; truncated: boolean; stopped: boolean };

/** "À voir plus tard" import from a public AniList list or a MyAnimeList export (watching, planning, on hold). */
export function AniListImport() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [user, setUser] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Result | null>(null);
  const controller = useRef<AbortController | null>(null);
  const [retry, setRetry] = useState<(() => void) | null>(null);

  /** Runs one import: `load` yields the AniList titles (from a username or a MAL file), then each is resolved to its franchise. */
  const importWith = async (load: (signal: AbortSignal) => Promise<AniListList>) => {
    if (busy) return;
    setRetry(() => () => void importWith(load));
    controller.current = new AbortController();
    const { signal } = controller.current;
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const list = await load(signal);
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

  const run = (event: FormEvent) => {
    event.preventDefault();
    if (user.trim()) void importWith((signal) => fetchAniListList(user, signal));
  };

  const onFile = (file: File | undefined) => {
    if (!file) return;
    void importWith(async (signal) => {
      // MAL offers its export as .xml.gz: unzip in the browser.
      const text = file.name.toLowerCase().endsWith(".gz")
        ? await new Response(file.stream().pipeThrough(new DecompressionStream("gzip"))).text()
        : await file.text();
      const ids = parseMalExport(text);
      if (!ids.length) return { entries: [], truncated: false };
      return fetchMalTitles(ids, signal);
    });
  };

  const close = () => { controller.current?.abort(); setOpen(false); setResult(null); setError(null); };
  if (!open) return <button type="button" className="clay clay-secondary clay-sm" onClick={() => setOpen(true)}><Download size={16} aria-hidden="true" />{t("Importer depuis AniList ou MyAnimeList")}</button>;
  return (
    <form onSubmit={run} className="import-panel">
      <label htmlFor="anilist-user">{t("Pseudo AniList (liste publique)")}</label>
      <div className="import-row">
        <div className="field-pill"><input id="anilist-user" value={user} onChange={(e) => setUser(e.target.value)} maxLength={20} autoComplete="off" disabled={busy} /></div>
        <button type="submit" className="clay clay-primary clay-sm" disabled={busy || !user.trim()}>
          {busy ? <><Loader2 size={14} className="animate-spin" aria-hidden="true" />{progress ? `${progress.done} / ${progress.total}` : "…"}</> : t("Importer")}
        </button>
        <button type="button" className="clay clay-secondary clay-sm" onClick={close}>{busy ? t("Annuler") : t("Fermer")}</button>
      </div>
      {busy && progress && <div className="import-track" role="progressbar" aria-label={t("Import en cours")} aria-valuemin={0} aria-valuemax={progress.total} aria-valuenow={progress.done}><b style={{ width: `${progress.total ? Math.round((progress.done / progress.total) * 100) : 0}%` }} /></div>}
      <label htmlFor="mal-file">{t("ou un export MyAnimeList (.xml ou .xml.gz)")}</label>
      <input id="mal-file" type="file" accept=".xml,.gz,application/xml,application/gzip" disabled={busy} onChange={(e) => { onFile(e.target.files?.[0]); e.target.value = ""; }} />
      {error && <div className="import-alert" role="alert"><TriangleAlert size={16} aria-hidden="true" /><span>{error}</span>{retry && <button type="button" className="clay clay-secondary clay-sm" onClick={retry}>{t("Réessayer")}</button>}</div>}
      {result && (
        <p role="status" className="import-result">
          <span><Check size={16} aria-hidden="true" />{t("{count} anime ajoutés à votre liste.", { count: result.added })}</span>
          {result.truncated && <span>{t("Seuls les 100 premiers titres ont été lus.")}</span>}
          {result.skipped > 0 && <span>{t(result.stopped ? "{count} titres n’ont pas pu être résolus (AniList est limité), relancez l’import plus tard." : "{count} titres n’ont pas pu être résolus.", { count: result.skipped })}</span>}
        </p>
      )}
    </form>
  );
}
