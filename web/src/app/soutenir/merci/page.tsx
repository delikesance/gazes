"use client";
import Link from "next/link";
import { useI18n } from "@/lib/i18n";

export default function ThanksPage() {
  const { t } = useI18n();
  return (
    <main className="history-page">
      <div className="history-inner page-inset support-page">
        <span className="eyebrow">{t("Soutenir Gazes")}</span>
        <h1 className="serif">{t("Merci")}</h1>
        <p className="support-lead">{t("Votre don est bien parti. Le réseau crypto peut mettre quelques minutes à le confirmer : si vous avez choisi d’afficher un nom, il apparaîtra sur la page de soutien une fois le paiement confirmé.")}</p>
        <div className="hero-actions">
          <Link href="/" className="design-button">{t("Retour au catalogue")}</Link>
          <Link href="/soutenir" className="design-button secondary-button">{t("Voir le mur des donateurs")}</Link>
        </div>
      </div>
    </main>
  );
}
