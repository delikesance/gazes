"use client";
import { useI18n } from "@/lib/i18n";

import { useEffect, useRef } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { ThemeToggle } from "./ThemeToggle";
import { AccountMenu } from "./AccountMenu";
import { HeaderNav, HeaderResume } from "./HeaderNav";

function Header() {
  const { t } = useI18n();
  const router = useRouter();
  const pathname = usePathname();
  const breadcrumbSlot = useRef<HTMLDivElement>(null);
  useEffect(() => { if (breadcrumbSlot.current) { breadcrumbSlot.current.dataset.ready = "true"; window.dispatchEvent(new Event("gazes-header-ready")); } }, []);
  // "/" or Ctrl/Cmd+K opens the search page from anywhere outside a text field.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))) return;
      const combo = (event.ctrlKey || event.metaKey) && event.code === "KeyK";
      if (!combo && (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey)) return;
      event.preventDefault();
      router.push("/search");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router]);
  return <header className="catalog-toolbar catalog-header page-inset" aria-label={t("Navigation principale")}>
    <Link href="/" className="site-wordmark" aria-label={t("Gazes, accueil")}>gazes<span>.</span></Link>
    <HeaderNav />
    <div ref={breadcrumbSlot} id="header-breadcrumb" className="header-breadcrumb-slot" />
    <div className="header-actions">
      <HeaderResume />
      <Link href="/search" className="header-search-toggle" aria-label={t("Rechercher")} aria-current={pathname === "/search" ? "page" : undefined}><Search size={18} aria-hidden="true" /></Link>
      <ThemeToggle />
      <AccountMenu />
    </div>
  </header>;
}

export function SiteHeaderFallback() { return <Header />; }

export function SiteHeader() { return <Header />; }
