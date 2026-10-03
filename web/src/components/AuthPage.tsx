"use client";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { AtSign, Eye, EyeOff, Loader2, Lock, Mail, ShieldCheck } from "lucide-react";
import { AuthError, solveFreshCaptcha, type AuthErrorCode } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "./AuthProvider";
import { PageGrid } from "./ui/PageGrid";
import { Scribble } from "./ui/Scribble";

type Mode = "login" | "register";

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const PSEUDO = /^[\p{L}\p{N}_.-]{3,24}$/u;

const ERRORS: Record<string, string> = {
  invalid_email: "Adresse e-mail invalide.",
  invalid_pseudo: "Le pseudo doit faire 3 à 24 caractères (lettres, chiffres, _ . -).",
  invalid_password: "Le mot de passe doit faire au moins 8 caractères.",
  invalid_credentials: "E-mail ou mot de passe incorrect.",
  email_taken: "Cette adresse e-mail est déjà utilisée.",
  captcha_failed: "Vérification anti-robot échouée. Réessayez.",
  rate_limited: "Trop de tentatives. Réessayez dans quelques minutes.",
  network: "Impossible de joindre le serveur.",
  terms: "Vous devez accepter les conditions d’utilisation.",
};

/** 0 to 4: length, length+, letter case mix, digit/symbol. */
export function passwordStrength(password: string): number {
  let score = 0;
  if (password.length >= 8) score++;
  if (password.length >= 12) score++;
  if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score++;
  if (/\d/.test(password) && /[^\p{L}\p{N}]/u.test(password)) score++;
  return score;
}

function safeNext(raw: string | null): string {
  return raw && raw.startsWith("/") && !raw.startsWith("//") ? raw : "/";
}

function Field({ id, label, icon, error, aside, children }: { id: string; label: string; icon: ReactNode; error?: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <div className="auth-field">
      <label htmlFor={id} className="auth-label"><span>{label}</span>{aside}</label>
      <div className="field-pill" data-invalid={Boolean(error)}>{icon}{children}</div>
      {error && <p className="field-error" role="alert">{error}</p>}
    </div>
  );
}

export function AuthPage({ mode }: { mode: Mode }) {
  const { t } = useI18n();
  const { user, login, register } = useAuth();
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"));
  const isRegister = mode === "register";

  const [email, setEmail] = useState("");
  const [pseudo, setPseudo] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [terms, setTerms] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);
  const strength = useMemo(() => passwordStrength(password), [password]);

  useEffect(() => { if (user) router.replace(next); }, [user, next, router]);

  const translateCode = (code: AuthErrorCode | string) => t(ERRORS[code] || "Une erreur est survenue. Réessayez.");

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const found: Record<string, string> = {};
    if (!EMAIL.test(email.trim())) found.email = translateCode("invalid_email");
    if (isRegister && !PSEUDO.test(pseudo.trim())) found.pseudo = translateCode("invalid_pseudo");
    if (password.length < 8) found.password = translateCode("invalid_password");
    if (isRegister && !terms) found.terms = translateCode("terms");
    setErrors(found);
    setFormError("");
    if (Object.keys(found).length) return;
    setBusy(true);
    try {
      // The proof-of-work runs when the form is sent; each solution is single-use.
      const captcha = await solveFreshCaptcha();
      if (isRegister) await register({ email: email.trim(), pseudo: pseudo.trim(), password, captcha });
      else await login({ email: email.trim(), password, captcha });
      router.replace(next);
    } catch (error) {
      const code = error instanceof AuthError ? error.code : (error as Error).message === "captcha_failed" ? "captcha_failed" : "server_error";
      if (code === "email_taken") setErrors({ email: translateCode(code) });
      else if (code === "invalid_email" || code === "invalid_pseudo" || code === "invalid_password") setErrors({ [code.replace("invalid_", "")]: translateCode(code) });
      else setFormError(translateCode(code));
    } finally {
      setBusy(false);
    }
  };

  const other = isRegister ? "/login" : "/register";
  const otherHref = next === "/" ? other : `${other}?next=${encodeURIComponent(next)}`;

  return (
    <main className="auth-page">
      <PageGrid />
      <section className="auth-card" aria-labelledby="auth-title">
        <div className="auth-pitch">
          <Scribble shape="a" width={300} rotate={-8} style={{ left: -70, top: -60 }} />
          <Scribble shape="b" width={320} rotate={6} style={{ right: -90, bottom: -40 }} />
          <div className="auth-pitch-copy">
            <h2 className="serif">{t(isRegister ? "Gardez votre progression partout." : "Reprenez là où vous vous êtes arrêté.")}</h2>
            <p>{t(isRegister ? "Un compte, c’est votre historique de lecture sur tous vos appareils." : "Votre progression est enregistrée et vous suit d’un appareil à l’autre.")}</p>
          </div>
        </div>

        <form className="auth-form" onSubmit={submit} noValidate>
          <div className="auth-heading">
            <span className="eyebrow">{t(isRegister ? "Inscription" : "Connexion")}</span>
            <h1 id="auth-title" className="serif">{t(isRegister ? "Créer un compte" : "Se connecter")}</h1>
          </div>

          {isRegister && (
            <Field id="pseudo" label={t("Pseudo")} icon={<AtSign size={16} aria-hidden="true" />} error={errors.pseudo}>
              <input id="pseudo" name="pseudo" autoComplete="username" placeholder={t("votrepseudo")} value={pseudo} onChange={(e) => setPseudo(e.target.value)} maxLength={24} />
            </Field>
          )}
          <Field id="email" label={t("Adresse e-mail")} icon={<Mail size={16} aria-hidden="true" />} error={errors.email}>
            <input id="email" name="email" type="email" autoComplete="email" placeholder={t("vous@exemple.com")} value={email} onChange={(e) => setEmail(e.target.value)} maxLength={254} />
          </Field>
          <Field
            id="password"
            label={t("Mot de passe")}
            icon={<Lock size={16} aria-hidden="true" />}
            error={errors.password}
          >
            <input id="password" name="password" type={showPassword ? "text" : "password"} autoComplete={isRegister ? "new-password" : "current-password"} placeholder="••••••••" value={password} onChange={(e) => setPassword(e.target.value)} maxLength={256} />
            <button type="button" className="field-toggle" aria-label={t(showPassword ? "Masquer le mot de passe" : "Afficher le mot de passe")} aria-pressed={showPassword} onClick={() => setShowPassword((v) => !v)}>
              {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          </Field>

          {isRegister && (
            <div className="strength" aria-live="polite">
              <div className="strength-bars" data-score={strength}>{[0, 1, 2, 3].map((i) => <span key={i} data-on={i < strength} />)}</div>
              <span>{t("8 caractères minimum.")}</span>
            </div>
          )}


          {isRegister && (
            <>
              <label className="terms">
                <input type="checkbox" checked={terms} onChange={(e) => setTerms(e.target.checked)} />
                <span className="terms-box" aria-hidden="true" />
                <span>{t("J’accepte les conditions d’utilisation")}</span>
              </label>
              {errors.terms && <p className="field-error" role="alert">{errors.terms}</p>}
            </>
          )}

          {formError && <p className="form-error" role="alert">{formError}</p>}
          <button type="submit" className="clay clay-primary clay-lg" disabled={busy} aria-busy={busy}>
            {busy ? <><Loader2 size={16} className="animate-spin" aria-hidden="true" />{t("Vérification…")}</> : t(isRegister ? "Créer mon compte" : "Se connecter")}
          </button>
          <p className="auth-note"><ShieldCheck size={14} aria-hidden="true" />{t("Vérification automatique à l’envoi, sans case à cocher.")}</p>
          <p className="auth-switch">
            {t(isRegister ? "Déjà un compte ?" : "Pas encore de compte ?")} <Link href={otherHref}>{t(isRegister ? "Se connecter" : "S’inscrire")}</Link>
          </p>
        </form>
      </section>
    </main>
  );
}
