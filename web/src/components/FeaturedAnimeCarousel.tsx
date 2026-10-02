"use client";

import { useEffect, useRef, useState, type CSSProperties } from "react";
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
  const [mobile, setMobile] = useState(false);
  const touchStart = useRef<{ x: number; y: number } | null>(null);

  useEffect(() => {
    const query = window.matchMedia("(max-width: 600px)");
    const update = () => { setMobile(query.matches); touchStart.current = null; };
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  if (!items.length) return null;
  const activeIndex = index % items.length;
  // The ring around the play/pause button is the rotation timer: when it completes, the next slide shows.
  function advance() {
    if (mobile) return;
    setDirection(1);
    setIndex(current => (current + 1) % items.length);
  }
  function move(offset: number) {
    setDirection(offset > 0 ? 1 : -1);
    setIndex(current => (current + offset + items.length) % items.length);
    setPaused(true);
  }

  return <div className="featured-carousel" style={{ "--dir": direction } as CSSProperties}
    onTouchStart={event => {
      if (!mobile || event.touches.length !== 1 || (event.target as HTMLElement).closest("button, a, input")) {
        touchStart.current = null;
        return;
      }
      touchStart.current = { x: event.touches[0].clientX, y: event.touches[0].clientY };
    }}
    onTouchCancel={() => { touchStart.current = null; }}
    onTouchEnd={event => {
      const start = touchStart.current;
      touchStart.current = null;
      if (!mobile || !start || !event.changedTouches.length || items.length < 2) return;
      const dx = event.changedTouches[0].clientX - start.x;
      const dy = event.changedTouches[0].clientY - start.y;
      if (Math.abs(dx) >= 50 && Math.abs(dx) > Math.abs(dy) * 1.25) move(dx < 0 ? 1 : -1);
    }}
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
          <circle key={activeIndex} className="featured-ring-progress" cx="18" cy="18" r="17" pathLength={100} data-paused={mobile || paused || interacting} onAnimationEnd={event => { if (event.animationName === "hero-ring") advance(); }} />
        </svg>
        {paused ? <Play size={15} /> : <Pause size={15} />}
      </button>
    </div>}
    {items.length > 1 && <div className="featured-dots" aria-label={t("À la une")}>
      {items.map((item, dotIndex) => <button key={item.media_id || item.id}
        type="button"
        aria-label={`${t("À la une")} : ${item.title_romaji || item.display_title}`}
        aria-current={dotIndex === activeIndex ? "true" : undefined}
        onClick={() => move(dotIndex - activeIndex)}
      ><span /></button>)}
    </div>}
  </div>;
}
