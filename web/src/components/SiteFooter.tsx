"use client";
import { setLocale, type Locale, useI18n } from "@/lib/i18n";
import { Select } from "./ui/Select";
import Link from "next/link";
import { ArrowUp } from "lucide-react";

export function SiteFooter() {
  const { t, locale } = useI18n();
  return <footer className="site-footer page-inset">
    <div className="footer-identity"><Link href="/" className="footer-wordmark">gazes<span>.</span></Link><p>© {new Date().getFullYear()} {" "}{t("Gazes. Tous droits réservés.")}</p><p className="footer-disclaimer">{t("Gazes n’héberge pas de vidéos de façon permanente. Les flux proviennent de sources externes.")}</p></div>
    <nav aria-label={t("Navigation de pied de page")}><Link href="/">{t("Catalogue")}</Link><Link href="/genre/action">{t("Genres")}</Link><Link href="/changelog">{t("Nouveautés")}</Link><Link href="/soutenir">{t("Soutenir")}</Link><Link href="/privacy">{t("Confidentialité")}</Link><Link href="/status">{t("État du service")}</Link><a href="#top">{t("Haut de page")}{" "}<ArrowUp size={14} aria-hidden="true" /></a><label className="footer-language"><span>{t("Langue")}</span><Select aria-label={t("Langue")} value={locale} onChange={event => setLocale(event.target.value as Locale)}><option value="fr">Français</option><option value="en">English</option></Select></label></nav>
  </footer>;
}
