"use client";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Clock, Compass, Sparkles } from "lucide-react";
import { useI18n } from "@/lib/i18n";

/** Thumb-reach navigation, phones only (hidden from 768px by CSS) and never over the player. */
export function MobileNav() {
  const { t } = useI18n();
  const pathname = usePathname();
  const forYou = useSearchParams().get("tab") === "suggestions";
  if (pathname.includes("/episodes/")) return null;
  const items = [
    { href: "/", label: t("Catalogue"), icon: Compass, active: pathname === "/" && !forYou },
    { href: "/?tab=suggestions", label: t("Pour vous"), icon: Sparkles, active: pathname === "/" && forYou },
    { href: "/history", label: t("Historique"), icon: Clock, active: pathname === "/history" },
  ];
  return <nav className="mobile-nav" aria-label={t("Navigation principale")}>
    {items.map(({ href, label, icon: Icon, active }) => <Link key={href} href={href} aria-current={active ? "page" : undefined}><Icon size={20} aria-hidden="true" /><span>{label}</span></Link>)}
  </nav>;
}
