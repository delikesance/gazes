"use client";
import { useI18n } from "@/lib/i18n";

import { useEffect, useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowRight, Search, X } from "lucide-react";
import { ThemeToggle } from "./ThemeToggle";

function Header({ query = "", onSubmit }: { query?: string; onSubmit?: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  const [searchOpen,setSearchOpen]=useState(false);
  const breadcrumbSlot=useRef<HTMLDivElement>(null);
  useEffect(()=>{if(breadcrumbSlot.current){breadcrumbSlot.current.dataset.ready="true";window.dispatchEvent(new Event("gazes-header-ready"));}},[]);
  const searchButton=useRef<HTMLButtonElement>(null);
  const closeSearch=()=>{setSearchOpen(false);searchButton.current?.focus();};
  return <header className={`catalog-toolbar catalog-header page-inset${searchOpen?" search-open":""}`} aria-label={t("Navigation principale")}>
    <Link href="/" className="site-wordmark" aria-label={t("Gazes, accueil")}>gazes<span>.</span></Link>
    <div ref={breadcrumbSlot} id="header-breadcrumb" className="header-breadcrumb-slot" />
    <div className="header-actions">
      <button ref={searchButton} type="button" className="header-search-toggle" aria-label={t("Rechercher")} aria-expanded={searchOpen} aria-controls="header-search-form" onClick={()=>setSearchOpen(true)}><Search size={18} aria-hidden="true" /></button>
      {searchOpen&&<form id="header-search-form" key={query} action="/" method="get" onSubmit={event=>{onSubmit?.(event);if(onSubmit)setSearchOpen(false);}} className="catalog-search" role="search" onKeyDown={event=>{if(event.key==="Escape"){event.preventDefault();closeSearch();}}} onBlur={event=>{if(!event.currentTarget.contains(event.relatedTarget))setSearchOpen(false);}}>
        <Search size={18} aria-hidden="true" />
        <input autoFocus name="q" aria-label={t("Rechercher un anime")} defaultValue={query} placeholder={t("Rechercher un anime…")} />
        <button type="submit" aria-label={t("Rechercher")}><ArrowRight size={19} /></button>
        <button type="button" aria-label={t("Fermer la recherche")} onClick={closeSearch}><X size={17} /></button>
      </form>}
      <ThemeToggle />
    </div>
  </header>;
}

export function SiteHeaderFallback() { return <Header />; }

export function SiteHeader() {
  const router = useRouter();
  const [query, setQuery] = useState("");

  useEffect(() => {
    if (typeof window !== "undefined") {
      setQuery(new URLSearchParams(window.location.search).get("q") || "");
    }
  }, []);

  return <Header query={query} onSubmit={event => {
    event.preventDefault();
    const q = String(new FormData(event.currentTarget).get("q") || "").trim();
    const search = new URLSearchParams({q, page:"1"});
    router.push(q ? `/?${search.toString()}` : "/");
  }} />;
}
