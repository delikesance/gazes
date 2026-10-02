"use client";
import Link from "next/link";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "./AuthProvider";
import { Scribble } from "./ui/Scribble";

/** Invitation to create an account; only shown to signed-out visitors. */
export function AccountCta() {
  const { t } = useI18n();
  const { user } = useAuth();
  if (user !== null) return null;
  return (
    <section className="account-cta page-inset" aria-labelledby="account-cta-title">
      <div className="account-cta-card">
        <Scribble shape="a" width={300} rotate={-8} style={{ left: -70, top: -60 }} />
        <Scribble shape="b" width={340} rotate={6} style={{ right: -60, bottom: -70 }} />
        <div className="account-cta-copy">
          <h2 id="account-cta-title" className="serif">{t("Gardez votre progression partout.")}</h2>
          <p>{t("Créez un compte pour retrouver votre historique et reprendre là où vous vous êtes arrêté, sur tous vos appareils.")}</p>
        </div>
        <div className="account-cta-actions">
          <Link href="/register" className="clay clay-primary clay-lg">{t("S’inscrire")}</Link>
          <Link href="/login" className="clay clay-secondary clay-lg">{t("Se connecter")}</Link>
        </div>
      </div>
    </section>
  );
}
