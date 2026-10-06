"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { useResume } from "@/lib/use-resume";

/** Primary navigation of the header (tablets and computers; phones use the bottom bar). */
export function HeaderNav() {
  const { t } = useI18n();
  const pathname = usePathname();
  const items = [
    { href: "/", label: t("Catalogue"), active: pathname === "/" },
    { href: "/for-you", label: t("Pour vous"), active: pathname === "/for-you" },
    { href: "/history", label: t("Bibliothèque"), active: pathname === "/history" },
  ];
  return <nav className="header-nav" aria-label={t("Navigation principale")}>
    {items.map(({ href, label, active }) => <Link key={href} href={href} aria-current={active ? "page" : undefined}>{label}</Link>)}
  </nav>;
}

/** One-click resume of the latest episode, on wide screens, from any page. */
export function HeaderResume() {
  const { t } = useI18n();
  const resume = useResume();
  if (!resume) return null;
  return <Link href={resume.href} className="header-resume" title={`${t(resume.next ? "Épisode suivant" : "Reprendre")} · ${resume.title}`}>
    <Play size={14} fill="currentColor" aria-hidden="true" />
    <span className="header-resume-title">{t(resume.next ? "Épisode suivant" : "Reprendre")} · {resume.title}</span>
    <span className="header-resume-ep">{t("Épisode")} {resume.episode}</span>
  </Link>;
}
