"use client";
import { useI18n } from "@/lib/i18n";

import { useEffect, useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowRight, Minus, Plus, Search, SlidersHorizontal, X } from "lucide-react";
import { GENRES, parseList } from "@/lib/genres";
import { ThemeToggle } from "./ThemeToggle";
import { AccountMenu } from "./AccountMenu";

function Header({ query = "", initialGenre = "", initialExclude = "", onSubmit, onClear }: { query?: string; initialGenre?: string; initialExclude?: string; onSubmit?: (event: FormEvent<HTMLFormElement>) => void; onClear?: () => void }) {
  const { t } = useI18n();
  const [searchOpen,setSearchOpen]=useState(false);
  const breadcrumbSlot=useRef<HTMLDivElement>(null);
  useEffect(()=>{if(breadcrumbSlot.current){breadcrumbSlot.current.dataset.ready="true";window.dispatchEvent(new Event("gazes-header-ready"));}},[]);
  // null until the viewer picks one here: the URL genre is the starting point.
  const [picked,setPicked]=useState<{include:string[];exclude:string[]}|null>(null);
  const include=picked?.include??parseList(initialGenre);
  const exclude=picked?.exclude??parseList(initialExclude);
  const hasFilter=include.length>0||exclude.length>0;
  // Left click wants a genre, right click refuses it; clicking again on the same side clears it.
  // Long press = exclude, for touch screens without a right click (iOS included): pointer events fire everywhere.
  const pressTimer=useRef<ReturnType<typeof setTimeout>>(undefined);
  const longPressed=useRef(false);
  const cancelPress=()=>{if(pressTimer.current){clearTimeout(pressTimer.current);pressTimer.current=undefined;}};
  const startPress=(value:string)=>{longPressed.current=false;cancelPress();pressTimer.current=setTimeout(()=>{longPressed.current=true;pressTimer.current=undefined;choose(value,"exclude");if(typeof navigator!=="undefined")navigator.vibrate?.(15);},450);};
  const choose=(value:string,side:"include"|"exclude")=>{
    const without=(list:string[])=>list.filter(item=>item!==value);
    const alreadyThere=(side==="include"?include:exclude).includes(value);
    setPicked({include:side==="include"&&!alreadyThere?[...without(include),value]:without(include),exclude:side==="exclude"&&!alreadyThere?[...without(exclude),value]:without(exclude)});
  };
  const [filterOpen,setFilterOpen]=useState(false);
  // On a search results page the bar stays open; elsewhere it opens on demand.
  const onSearchPage=Boolean(query||initialGenre||initialExclude);
  const searchVisible=searchOpen||onSearchPage;
  const formRef=useRef<HTMLFormElement>(null);
  const searchButton=useRef<HTMLButtonElement>(null);
  const filterButton=useRef<HTMLButtonElement>(null);
  // Every way of closing the search (X, Escape, blur, submit) must also reset the filter panel and the picked genre.
  const resetSearch=()=>{setSearchOpen(false);setFilterOpen(false);setPicked(null);};
  const closeSearch=()=>{resetSearch();searchButton.current?.focus();};
  // "/" or Ctrl/Cmd+K opens the search from anywhere outside a text field.
  useEffect(()=>{
    const onKey=(event:KeyboardEvent)=>{
      const target=event.target as HTMLElement|null;
      if(target&&(target.isContentEditable||["INPUT","TEXTAREA","SELECT"].includes(target.tagName)))return;
      const combo=(event.ctrlKey||event.metaKey)&&event.code==="KeyK";
      if(!combo&&(event.key!=="/"||event.ctrlKey||event.metaKey||event.altKey))return;
      event.preventDefault();
      setSearchOpen(true);
    };
    window.addEventListener("keydown",onKey);
    return()=>window.removeEventListener("keydown",onKey);
  },[]);
  // The phone tab bar opens the search from the thumb zone.
  useEffect(()=>{
    const open=()=>setSearchOpen(true);
    window.addEventListener("gazes-open-search",open);
    return()=>window.removeEventListener("gazes-open-search",open);
  },[]);
  // Tapping outside closes the search and the filters. Pointer position, not focus: Safari never focuses a tapped button.
  useEffect(()=>{
    if(!searchVisible)return;
    const onPointerDown=(event:PointerEvent)=>{if(formRef.current&&!formRef.current.contains(event.target as Node)&&!searchButton.current?.contains(event.target as Node))resetSearch();};
    document.addEventListener("pointerdown",onPointerDown);
    return()=>document.removeEventListener("pointerdown",onPointerDown);
  });
  return <header className={`catalog-toolbar catalog-header page-inset${searchVisible?" search-open":""}`} aria-label={t("Navigation principale")}>
    <Link href="/" className="site-wordmark" aria-label={t("Gazes, accueil")}>gazes<span>.</span></Link>
    <div ref={breadcrumbSlot} id="header-breadcrumb" className="header-breadcrumb-slot" />
    <div className="header-actions">
      <button ref={searchButton} type="button" className="header-search-toggle" aria-label={t("Rechercher")} aria-expanded={searchVisible} aria-controls="header-search-form" onClick={()=>setSearchOpen(true)}><Search size={18} aria-hidden="true" /></button>
      {searchVisible&&<form ref={formRef} id="header-search-form" key={query} action="/" method="get" onSubmit={event=>{onSubmit?.(event);if(onSubmit)resetSearch();}} className="catalog-search" role="search" onKeyDown={event=>{if(event.key==="Escape"){event.preventDefault();if(filterOpen){setFilterOpen(false);filterButton.current?.focus();}else closeSearch();}}} onBlur={event=>{if(event.relatedTarget&&!event.currentTarget.contains(event.relatedTarget as Node))resetSearch();}}>
        <Search size={18} aria-hidden="true" />
        <input autoFocus={searchOpen} name="q" aria-label={t("Rechercher un anime")} defaultValue={query} placeholder={t("Rechercher un anime…")} />
        <input type="hidden" name="genres" value={include.join(",")} />
        <input type="hidden" name="exclude" value={exclude.join(",")} />
        <button ref={filterButton} type="button" className="search-filter-toggle" data-active={hasFilter?"true":"false"} aria-label={t("Filtrer par genre")} aria-expanded={filterOpen} aria-controls="search-filter-panel" onClick={()=>setFilterOpen(open=>!open)}><SlidersHorizontal size={17} aria-hidden="true" /></button>
        <button type="submit" aria-label={t("Rechercher")}><ArrowRight size={19} /></button>
        <button type="button" aria-label={t(onSearchPage?"Effacer la recherche":"Fermer la recherche")} onClick={()=>{if(onSearchPage&&onClear){resetSearch();onClear();}else closeSearch();}}><X size={17} /></button>
        {filterOpen&&<div id="search-filter-panel" className="search-filter-panel" role="group" aria-label={t("Genres")}>
          <p className="search-filter-title">{t("Genres")}</p>
          <p className="search-filter-hint">{t("Clic gauche : inclure · Clic droit ou appui long : exclure")}</p>
          <div className="search-filter-options">
            {GENRES.map(([value,label])=>{
              const state=include.includes(value)?"include":exclude.includes(value)?"exclude":"none";
              const name=t(label);
              return <button key={value} type="button" data-state={state} aria-pressed={state!=="none"}
                aria-label={state==="include"?t("{genre}, inclus",{genre:name}):state==="exclude"?t("{genre}, exclu",{genre:name}):name}
                onPointerDown={event=>{if(event.button===0)startPress(value);}} onPointerUp={cancelPress} onPointerLeave={cancelPress} onPointerCancel={cancelPress}
                onClick={()=>{if(longPressed.current){longPressed.current=false;return;}choose(value,"include");}} onContextMenu={event=>{event.preventDefault();if(!longPressed.current)choose(value,"exclude");}}>
                {state==="include"&&<Plus size={13} aria-hidden="true" />}{state==="exclude"&&<Minus size={13} aria-hidden="true" />}{name}
              </button>;
            })}
          </div>
          <div className="search-filter-actions">
            <button type="button" className="search-filter-clear" disabled={!hasFilter} onClick={()=>setPicked({include:[],exclude:[]})}>{t("Effacer")}</button>
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
  const genre = params.get("genres") || params.get("genre") || "";
  const exclude = params.get("exclude") || "";

  return <Header query={query} initialGenre={genre} initialExclude={exclude} onClear={() => router.push("/")} onSubmit={event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const q = String(form.get("q") || "").trim();
    const genres = String(form.get("genres") || "");
    const exclude = String(form.get("exclude") || "");
    const search = new URLSearchParams({page:"1"});
    if (q) search.set("q", q);
    if (genres) search.set("genres", genres);
    if (exclude) search.set("exclude", exclude);
    router.push(q || genres || exclude ? `/?${search.toString()}` : "/");
  }} />;
}
