"use client";
import { useI18n } from "@/lib/i18n";

import { useEffect, useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowRight, Search, SlidersHorizontal, X } from "lucide-react";
import { ThemeToggle } from "./ThemeToggle";
import { AccountMenu } from "./AccountMenu";

// AniList genre names (what the catalogue search filters on) with their French labels.
const GENRES: [string, string][] = [
  ["Action", "Action"], ["Adventure", "Aventure"], ["Comedy", "Comédie"], ["Drama", "Drame"], ["Ecchi", "Ecchi"], ["Fantasy", "Fantastique"],
  ["Horror", "Horreur"], ["Mahou Shoujo", "Magical girl"], ["Mecha", "Mecha"], ["Music", "Musique"], ["Mystery", "Mystère"], ["Psychological", "Psychologique"],
  ["Romance", "Romance"], ["Sci-Fi", "Science-fiction"], ["Slice of Life", "Tranche de vie"], ["Sports", "Sport"], ["Supernatural", "Surnaturel"], ["Thriller", "Thriller"],
];

function Header({ query = "", initialGenre = "", onSubmit }: { query?: string; initialGenre?: string; onSubmit?: (event: FormEvent<HTMLFormElement>) => void }) {
  const { t } = useI18n();
  const [searchOpen,setSearchOpen]=useState(false);
  const breadcrumbSlot=useRef<HTMLDivElement>(null);
  useEffect(()=>{if(breadcrumbSlot.current){breadcrumbSlot.current.dataset.ready="true";window.dispatchEvent(new Event("gazes-header-ready"));}},[]);
  // null until the viewer picks one here: the URL genre is the starting point.
  const [picked,setGenre]=useState<string|null>(null);
  const genre=picked??initialGenre;
  const [filterOpen,setFilterOpen]=useState(false);
  const searchButton=useRef<HTMLButtonElement>(null);
  // Every way of closing the search (X, Escape, blur, submit) must also reset the filter panel and the picked genre.
  const resetSearch=()=>{setSearchOpen(false);setFilterOpen(false);setGenre(null);};
  const closeSearch=()=>{resetSearch();searchButton.current?.focus();};
  return <header className={`catalog-toolbar catalog-header page-inset${searchOpen?" search-open":""}`} aria-label={t("Navigation principale")}>
    <Link href="/" className="site-wordmark" aria-label={t("Gazes, accueil")}>gazes<span>.</span></Link>
    <div ref={breadcrumbSlot} id="header-breadcrumb" className="header-breadcrumb-slot" />
    <div className="header-actions">
      <button ref={searchButton} type="button" className="header-search-toggle" aria-label={t("Rechercher")} aria-expanded={searchOpen} aria-controls="header-search-form" onClick={()=>setSearchOpen(true)}><Search size={18} aria-hidden="true" /></button>
      {searchOpen&&<form id="header-search-form" key={query} action="/" method="get" onSubmit={event=>{onSubmit?.(event);if(onSubmit)resetSearch();}} className="catalog-search" role="search" onKeyDown={event=>{if(event.key==="Escape"){event.preventDefault();if(filterOpen)setFilterOpen(false);else closeSearch();}}} onBlur={event=>{if(!event.currentTarget.contains(event.relatedTarget))resetSearch();}}>
        <Search size={18} aria-hidden="true" />
        <input autoFocus name="q" aria-label={t("Rechercher un anime")} defaultValue={query} placeholder={t("Rechercher un anime…")} />
        <input type="hidden" name="genre" value={genre} />
        <button type="button" className="search-filter-toggle" data-active={genre?"true":"false"} aria-label={t("Filtrer par genre")} aria-expanded={filterOpen} aria-controls="search-filter-panel" onClick={()=>setFilterOpen(open=>!open)}><SlidersHorizontal size={17} aria-hidden="true" /></button>
        <button type="submit" aria-label={t("Rechercher")}><ArrowRight size={19} /></button>
        <button type="button" aria-label={t("Fermer la recherche")} onClick={closeSearch}><X size={17} /></button>
        {filterOpen&&<div id="search-filter-panel" className="search-filter-panel" role="group" aria-label={t("Genres")}>
          <p className="search-filter-title">{t("Genres")}</p>
          <div className="search-filter-options">
            {GENRES.map(([value,label])=><button key={value} type="button" aria-pressed={genre===value} onClick={()=>setGenre(genre===value?"":value)}>{t(label)}</button>)}
          </div>
          <div className="search-filter-actions">
            <button type="button" className="search-filter-clear" disabled={!genre} onClick={()=>setGenre("")}>{t("Effacer")}</button>
            <button type="submit" className="search-filter-apply">{t("Appliquer")}</button>
          </div>
        </div>}
      </form>}
      <ThemeToggle />
      <AccountMenu />
    </div>
  </header>;
}

export function SiteHeaderFallback() { return <Header />; }

export function SiteHeader() {
  const router = useRouter();
  // Read from the URL on every navigation, so the search box always reflects the page being shown.
  const params = useSearchParams();
  const query = params.get("q") || "";
  const genre = params.get("genre") || "";

  return <Header query={query} initialGenre={genre} onSubmit={event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const q = String(form.get("q") || "").trim();
    const genre = String(form.get("genre") || "");
    const search = new URLSearchParams({page:"1"});
    if (q) search.set("q", q);
    if (genre) search.set("genre", genre);
    router.push(q || genre ? `/?${search.toString()}` : "/");
  }} />;
}
