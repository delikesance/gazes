"use client";
import Link from "next/link";
import { useI18n } from "@/lib/i18n";
import { resultsBehind } from "@/lib/navigation-history";

/** First crumb of a series page: back to the results the viewer came from, else the catalogue. */
export function CatalogCrumb() {
  const { t } = useI18n();
  const results = resultsBehind();
  return <Link href={results?.url ?? "/"}>{results ? results.label || t("Résultats") : t("Catalogue")}</Link>;
}
