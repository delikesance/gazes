"use client";

import { useState, type CSSProperties } from "react";
import { ChevronLeft, ChevronRight, Pause, Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { AnimeCatalogItem } from "@/types/api";
import { FeaturedAnime } from "./AnimeHero";

export function FeaturedAnimeCarousel({ items }: { items: AnimeCatalogItem[] }) {
  const { t } = useI18n();
  const [index, setIndex] = useState(0);
  const [direction, setDirection] = useState<1 | -1>(1);
  const [paused, setPaused] = useState(false);
  const [interacting, setInteracting] = useState(false);

  if (!items.length) return null;
  const activeIndex = index % items.length;
  // The ring around the play/pause button is the rotation timer: when it completes, the next slide shows.
  function advance() {
    setDirection(1);
    setIndex(current => (current + 1) % items.length);
  }
  function move(offset: number) {
    setDirection(offset > 0 ? 1 : -1);
    setIndex(current => (current + offset + items.length) % items.length);
    setPaused(true);
  }

  return <div className="featured-carousel" style={{ "--dir": direction } as CSSProperties}
    onMouseEnter={() => setInteracting(true)}
    onMouseLeave={() => setInteracting(false)}
    onFocusCapture={() => setInteracting(true)}
    onBlurCapture={event => {
      if (!event.currentTarget.contains(event.relatedTarget)) setInteracting(false);
    }}>
    <FeaturedAnime key={items[activeIndex].media_id || items[activeIndex].id} anime={items[activeIndex]} />
    {items.length > 1 && <div className="featured-controls" aria-label={t("À la une")}>
      <button aria-label={t("Anime précédent")} onClick={() => move(-1)}><ChevronLeft size={18} /></button>
      <span>{activeIndex + 1} / {items.length}</span>
      <button aria-label={t("Anime suivant")} onClick={() => move(1)}><ChevronRight size={18} /></button>
      <button className="featured-toggle" aria-label={t(paused ? "Reprendre le défilement" : "Mettre le défilement en pause")} onClick={() => setPaused(current => !current)}>
        <svg className="featured-ring" viewBox="0 0 36 36" aria-hidden="true">
          <circle className="featured-ring-track" cx="18" cy="18" r="17" />
          <circle key={activeIndex} className="featured-ring-progress" cx="18" cy="18" r="17" pathLength={100} data-paused={paused || interacting} onAnimationEnd={event => { if (event.animationName === "hero-ring") advance(); }} />
        </svg>
        {paused ? <Play size={15} /> : <Pause size={15} />}
      </button>
    </div>}
  </div>;
}
