"use client";
import Link from "next/link";
import { useI18n } from "@/lib/i18n";

/** Body of the site-wide 404 (see app/not-found.tsx). */
export function NotFoundView() {
  const { t } = useI18n();
  return (
    <main className="history-page">
      <div className="history-inner page-inset account-page">
        <span className="eyebrow">404</span>
        <h1 className="serif">{t("Page introuvable")}</h1>
        <p className="history-empty">{t("Cette page n’existe pas ou n’est plus disponible.")}</p>
        <div className="hero-actions">
          <Link href="/" className="design-button">{t("Retour au catalogue")}</Link>
        </div>
      </div>
    </main>
  );
}
