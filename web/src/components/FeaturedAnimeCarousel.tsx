"use client";

import { useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, Pause, Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { AnimeCatalogItem } from "@/types/api";
import { FeaturedAnime } from "./AnimeHero";

export function FeaturedAnimeCarousel({ items }: { items: AnimeCatalogItem[] }) {
  const { t } = useI18n();
  const [index, setIndex] = useState(0);
  const [paused, setPaused] = useState(false);
  const [interacting, setInteracting] = useState(false);

  useEffect(() => {
    if (paused || interacting || items.length < 2) return;
    const timer = window.setInterval(() => {
      if (!document.hidden && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
        setIndex(current => (current + 1) % items.length);
      }
    }, 8000);
    return () => window.clearInterval(timer);
  }, [items.length, paused, interacting]);

  if (!items.length) return null;
  const activeIndex = index % items.length;
  function move(offset: number) {
    setIndex(current => (current + offset + items.length) % items.length);
    setPaused(true);
  }

  return <div className="featured-carousel"
    onMouseEnter={() => setInteracting(true)}
    onMouseLeave={() => setInteracting(false)}
    onFocusCapture={() => setInteracting(true)}
    onBlurCapture={event => {
      if (!event.currentTarget.contains(event.relatedTarget)) setInteracting(false);
    }}>
    <FeaturedAnime anime={items[activeIndex]} />
    {items.length > 1 && <div className="featured-controls" aria-label={t("À la une")}>
      <button aria-label={t("Anime précédent")} onClick={() => move(-1)}><ChevronLeft size={18} /></button>
      <span>{activeIndex + 1} / {items.length}</span>
      <button aria-label={t("Anime suivant")} onClick={() => move(1)}><ChevronRight size={18} /></button>
      <button aria-label={t(paused ? "Reprendre le défilement" : "Mettre le défilement en pause")} onClick={() => setPaused(current => !current)}>{paused ? <Play size={15} /> : <Pause size={15} />}</button>
    </div>}
  </div>;
}
