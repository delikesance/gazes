"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { AuthError, deleteAccount, listSessions, newRecoveryCodes, revokeSession, type AccountSession } from "@/lib/auth";
import { clearLocalWatchLog } from "@/lib/watch-log";
import { clearHidden } from "@/lib/hidden-anime";
import { exportMyData } from "@/lib/my-data";
import { useAuth } from "@/components/AuthProvider";
import { PageGrid } from "@/components/ui/PageGrid";
import { RecoveryCodes } from "@/components/RecoveryCodes";

export default function AccountPage() {
  const { t, locale } = useI18n();
  const { user, logout } = useAuth();
  const router = useRouter();
  const [sessions, setSessions] = useState<AccountSession[] | null>(null);
  const [password, setPassword] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [codesPassword, setCodesPassword] = useState("");
  const [codesOpen, setCodesOpen] = useState(false);

  const refresh = useCallback(() => { listSessions().then(setSessions, () => setSessions([])); }, []);
  useEffect(() => { if (user) refresh(); }, [user, refresh]);

  if (user === undefined) return <main className="history-page" />;
  if (user === null) {
    return (
      <main className="history-page">
        <div className="history-inner page-inset">
          <p className="history-empty">{t("Connectez-vous pour gérer votre compte.")} <Link href="/login">{t("Se connecter")}</Link></p>
        </div>
      </main>
    );
  }

  const onRevoke = async (id: string) => {
    try { await revokeSession(id); } catch { setError(t("Impossible de déconnecter cet appareil pour le moment.")); }
    refresh();
  };

  const onNewCodes = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      setCodes(await newRecoveryCodes(codesPassword));
      setCodesOpen(false);
      setCodesPassword("");
    } catch (err) {
      setError(err instanceof AuthError && err.code === "invalid_credentials" ? t("Mot de passe incorrect.") : t("Impossible de générer les codes pour le moment."));
    }
    setBusy(false);
  };

  const onDelete = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await deleteAccount(password);
      clearLocalWatchLog();
      clearHidden();
      await logout();
      router.replace("/");
      router.refresh();
    } catch (err) {
      setError(err instanceof AuthError && err.code === "invalid_credentials" ? t("Mot de passe incorrect.") : t("Impossible de supprimer le compte pour le moment."));
      setBusy(false);
    }
  };

  return (
    <main className="history-page">
      <PageGrid />
      <div className="history-inner page-inset account-page">
        <span className="eyebrow">{user.pseudo}</span>
        <h1 className="serif">{t("Mon compte")}</h1>

        <section className="account-section">
          <h2>{t("Appareils connectés")}</h2>
          {sessions === null && <p className="history-empty">…</p>}
          <ul className="account-sessions">
            {(sessions ?? []).map((session, index) => (
              <li key={session.id}>
                <span>
                  {session.current ? t("Cet appareil") : `${t("Appareil")} ${index + 1}`}
                  <small>{t("Dernière activité")} {new Date(session.last_seen * 1000).toLocaleDateString(locale)}</small>
                </span>
                {!session.current && <button type="button" className="clay clay-secondary clay-sm" onClick={() => onRevoke(session.id)}>{t("Déconnecter")}</button>}
              </li>
            ))}
          </ul>
        </section>

        <section className="account-section">
          <h2>{t("Codes de secours")}</h2>
          <p className="history-empty">{t("Ils permettent de retrouver votre compte si vous perdez votre mot de passe. En générer de nouveaux invalide les anciens.")}</p>
          {codes ? <RecoveryCodes codes={codes} doneLabel={t("J’ai conservé mes codes")} onDone={() => setCodes(null)} /> : !codesOpen ? (
            <div><button type="button" className="clay clay-secondary clay-sm" onClick={() => setCodesOpen(true)}>{t("Générer de nouveaux codes")}</button></div>
          ) : (
            <form onSubmit={onNewCodes} className="account-delete">
              <label htmlFor="codes-password">{t("Confirmez avec votre mot de passe")}</label>
              <div className="field-pill"><input id="codes-password" type="password" autoComplete="current-password" value={codesPassword} onChange={(e) => setCodesPassword(e.target.value)} maxLength={256} required /></div>
              <div className="account-delete-actions">
                <button type="submit" className="clay clay-primary clay-sm" disabled={busy || codesPassword.length === 0}>{t("Générer")}</button>
                <button type="button" className="clay clay-secondary clay-sm" onClick={() => { setCodesOpen(false); setCodesPassword(""); }}>{t("Annuler")}</button>
              </div>
            </form>
          )}
        </section>

        <section className="account-section">
          <h2>{t("Mes données")}</h2>
          <p className="history-empty">{t("Téléchargez tout ce que Gazes conserve sur votre compte.")}</p>
          <div><button type="button" className="clay clay-secondary clay-sm" onClick={() => exportMyData().catch(() => setError(t("Impossible d’exporter vos données pour le moment.")))}>{t("Exporter mes données")}</button></div>
        </section>

        <section className="account-section">
          <h2>{t("Supprimer mon compte")}</h2>
          <p className="history-empty">{t("Votre compte, votre historique, votre liste et vos réglages sont effacés définitivement. Cette action est irréversible.")}</p>
          {!confirming ? (
            <div><button type="button" className="clay clay-secondary clay-sm account-danger" onClick={() => setConfirming(true)}>{t("Supprimer mon compte")}</button></div>
          ) : (
            <form onSubmit={onDelete} className="account-delete">
              <label htmlFor="delete-password">{t("Confirmez avec votre mot de passe")}</label>
              <div className="field-pill">
                <input id="delete-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} maxLength={256} required />
              </div>
              <div className="account-delete-actions">
                <button type="submit" className="clay clay-primary clay-sm" disabled={busy || password.length === 0}>{t("Supprimer définitivement")}</button>
                <button type="button" className="clay clay-secondary clay-sm" onClick={() => { setConfirming(false); setPassword(""); }}>{t("Annuler")}</button>
              </div>
            </form>
          )}
        </section>
        {error && <p className="field-error" role="alert">{error}</p>}
      </div>
    </main>
  );
}
