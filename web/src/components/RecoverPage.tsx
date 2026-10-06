"use client";
import Link from "next/link";
import { useState, type FormEvent } from "react";
import { KeyRound, Loader2, Lock, Mail } from "lucide-react";
import { useRouter } from "next/navigation";
import { AuthError, solveFreshCaptcha } from "@/lib/auth";
import { useAuth } from "./AuthProvider";
import { useI18n } from "@/lib/i18n";
import { PageGrid } from "./ui/PageGrid";
import { Scribble } from "./ui/Scribble";

const ERRORS: Record<string, string> = {
  invalid_code: "Code de secours invalide ou déjà utilisé.",
  invalid_password: "Le mot de passe doit faire au moins 8 caractères.",
  captcha_failed: "Vérification anti-robot échouée. Réessayez.",
  rate_limited: "Trop de tentatives. Réessayez dans quelques minutes.",
  network: "Impossible de joindre le serveur.",
};

/** Sets a new password from the e-mail and one of the recovery codes given at sign-up. */
export function RecoverPage() {
  const { t } = useI18n();
  const { recover } = useAuth();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (password.length < 8) { setError(t(ERRORS.invalid_password)); return; }
    setBusy(true);
    setError("");
    try {
      const captcha = await solveFreshCaptcha();
      await recover({ email: email.trim(), code: code.trim(), password, captcha });
      router.replace("/");
    } catch (err) {
      const key = err instanceof AuthError ? err.code : (err as Error).message === "captcha_failed" ? "captcha_failed" : "network";
      setError(t(ERRORS[key] || "Une erreur est survenue. Réessayez."));
      setBusy(false);
    }
  };

  return (
    <main className="auth-page">
      <PageGrid />
      <section className="auth-card" aria-labelledby="auth-title">
        <div className="auth-pitch">
          <Scribble shape="a" width={300} rotate={-8} style={{ left: -70, top: -60 }} />
          <div className="auth-pitch-copy">
            <h2 className="serif">{t("Retrouvez votre compte.")}</h2>
            <p>{t("Utilisez l’un des codes de secours reçus à l’inscription. Chaque code ne sert qu’une fois.")}</p>
          </div>
        </div>
        <form className="auth-form" onSubmit={submit} noValidate>
          <div className="auth-heading">
            <span className="eyebrow">{t("Connexion")}</span>
            <h1 id="auth-title" className="serif">{t("Mot de passe oublié")}</h1>
          </div>
          <div className="auth-field">
            <label htmlFor="recover-email" className="auth-label"><span>{t("Adresse e-mail")}</span></label>
            <div className="field-pill"><Mail size={16} aria-hidden="true" /><input id="recover-email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} maxLength={254} required /></div>
          </div>
          <div className="auth-field">
            <label htmlFor="recover-code" className="auth-label"><span>{t("Code de secours")}</span></label>
            <div className="field-pill"><KeyRound size={16} aria-hidden="true" /><input id="recover-code" autoComplete="off" autoCapitalize="characters" placeholder="XXXX-XXXX-XXXX" value={code} onChange={(e) => setCode(e.target.value)} maxLength={20} required /></div>
          </div>
          <div className="auth-field">
            <label htmlFor="recover-password" className="auth-label"><span>{t("Nouveau mot de passe")}</span></label>
            <div className="field-pill"><Lock size={16} aria-hidden="true" /><input id="recover-password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} maxLength={256} required /></div>
          </div>
          {error && <p className="form-error" role="alert">{error}</p>}
          <button type="submit" className="clay clay-primary clay-lg" disabled={busy || !email || !code || !password} aria-busy={busy}>
            {busy ? <><Loader2 size={16} className="animate-spin" aria-hidden="true" />{t("Vérification…")}</> : t("Réinitialiser le mot de passe")}
          </button>
          <p className="auth-switch"><Link href="/login">{t("Retour à la connexion")}</Link></p>
        </form>
      </section>
    </main>
  );
}
