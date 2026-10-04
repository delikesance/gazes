"use client";
import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { trackRoute } from "@/lib/navigation-history";
import { useI18n } from "@/lib/i18n";

export function LocaleDocument() {
  const {locale, t} = useI18n();
  const pathname = usePathname();
  useEffect(() => { trackRoute(pathname); }, [pathname]);
  const title = t("Gazes — Découvrez votre prochain anime");
  const description = t("Découvrez les animes du moment, explorez leurs saisons et regardez vos épisodes sur Gazes.");
  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = title;
    document.querySelector('meta[name="description"]')?.setAttribute("content", description);
  }, [locale,title,description]);
  return null;
}

export function LoadingMessage() {
  const {t} = useI18n();
  return <p role="status">{t("Chargement…")}</p>;
}
