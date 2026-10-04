"use client";
import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { episodePreviewUrl } from "@/lib/api";
import type { EpisodeInfo } from "@/types/api";
import { Scribble } from "./ui/Scribble";

interface PlayerEpisodePickerProps {
  episodes: EpisodeInfo[];
  fallbackThumbnail?: string;
  /** Lets episodes without a provider still show the frame cut when they were first played. */
  seasonId?: number;
  currentEpisode?: number;
  onSelect: (episode: number) => void;
  onClose: () => void;
}

/** Frosted rail of episodes that opens above the control dock. */
export function PlayerEpisodePicker({ episodes, currentEpisode, seasonId, onSelect, onClose }: PlayerEpisodePickerProps) {
  const { t } = useI18n();
  const [failed, setFailed] = useState<Set<string>>(new Set());
  const railRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    railRef.current?.querySelector<HTMLElement>("[data-current='true']")?.scrollIntoView({ inline: "center", block: "nearest" });
  }, []);

  const scroll = (direction: number) => railRef.current?.scrollBy({ left: direction * railRef.current.clientWidth * 0.8, behavior: "smooth" });

  return (
    <div role="dialog" aria-label={t("Épisodes")} className="player-panel absolute inset-x-0 bottom-full mb-3 flex flex-col gap-4 overflow-hidden !p-5 animate-in fade-in duration-100" style={{ borderRadius: "var(--radius-panel)" }}>
      <Scribble shape="b" width={300} rotate={8} style={{ right: -80, top: -90 }} />
      <div className="relative flex items-center justify-between">
        <span className="player-panel-title !p-0">{t("Épisodes")} · {episodes.length}</span>
        <div className="flex gap-2">
          <button type="button" className="player-pill player-pill--sm player-pill--icon" aria-label={t("Épisodes précédents")} onClick={() => scroll(-1)}><ChevronLeft className="h-4 w-4" /></button>
          <button type="button" className="player-pill player-pill--sm player-pill--icon" aria-label={t("Épisodes suivants")} onClick={() => scroll(1)}><ChevronRight className="h-4 w-4" /></button>
        </div>
      </div>
      <div ref={railRef} className="relative -m-1 flex gap-4 overflow-x-auto p-1" style={{ scrollbarWidth: "none" }}>
        {episodes.map(episode => {
          const current = episode.episode_number === currentEpisode;
          const thumb = episode.thumbnail || (seasonId && !episode.upcoming ? episodePreviewUrl(seasonId, episode.episode_number) : undefined);
          return (
            <button
              key={episode.episode_number}
              type="button"
              data-current={current}
              disabled={episode.upcoming}
              aria-current={current ? "true" : undefined}
              onClick={() => onSelect(episode.episode_number)}
              className="player-episode flex w-[200px] shrink-0 flex-col gap-2 text-left disabled:opacity-40 sm:w-[212px]"
            >
              <span className="player-episode-thumb relative block aspect-video w-full overflow-hidden bg-zinc-800" style={{ borderRadius: "var(--radius-poster)" }}>
                {thumb && !failed.has(thumb) && (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={thumb} alt="" loading="lazy" className="h-full w-full object-cover" onError={() => setFailed(prev => new Set(prev).add(thumb))} />
                )}
                <span className="player-chip absolute left-2 top-2" style={{ background: "rgba(9,9,11,.6)" }}>EP {episode.episode_number}</span>
                {current && <span className="player-chip player-chip--solid absolute right-2 top-2">{t("En cours de lecture")}</span>}
              </span>
              <span className="truncate px-1 text-[13px] font-medium text-zinc-100">{episode.upcoming ? t("À venir") : episode.title || `${t("Épisode")} ${episode.episode_number}`}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
