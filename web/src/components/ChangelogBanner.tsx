"use client";
import { useSyncExternalStore } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Sparkles, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { CHANGELOG, announcedEntry } from "@/lib/changelog";

const SEEN_KEY = "gazes-changelog-seen";

const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  window.addEventListener("storage", listener);
  return () => { listeners.delete(listener); window.removeEventListener("storage", listener); };
}

/** Date of the entry to announce, "" when none (always "" on the server, so nothing hydrates differently). */
function announcedDate(): string {
  let seen: string | null = null;
  try { seen = localStorage.getItem(SEEN_KEY); } catch {}
  return announcedEntry(CHANGELOG, Date.now(), seen)?.date ?? "";
}

/** Records the latest entry as seen, so the banner stops announcing it. */
export function markChangelogSeen() {
  try { if (CHANGELOG[0]) localStorage.setItem(SEEN_KEY, CHANGELOG[0].date); } catch {}
  listeners.forEach((listener) => listener());
}

/** Announces the latest dev-log entry for a few days, until it is opened or dismissed. Never over the player. */
export function ChangelogBanner() {
  const { t, locale } = useI18n();
  const pathname = usePathname();
  const date = useSyncExternalStore(subscribe, announcedDate, () => "");
  const entry = CHANGELOG.find((e) => e.date === date);
  if (!entry || pathname === "/changelog" || pathname.includes("/episodes/")) return null;
  return <aside className="changelog-banner" aria-label={t("Nouveautés")}>
    <Sparkles size={16} aria-hidden="true" className="changelog-banner-icon" />
    <Link href="/changelog" onClick={markChangelogSeen}><span className="changelog-banner-label">{t("Nouveau")}</span>{" "}{entry[locale === "en" ? "en" : "fr"].title}</Link>
    <button type="button" onClick={markChangelogSeen} aria-label={t("Fermer l’annonce")}><X size={16} aria-hidden="true" /></button>
  </aside>;
}
