"use client";
import { useI18n } from "@/lib/i18n";

import React from "react";
import { AnimeGroup, TorrentItem } from "@/types/api";
import { LazyImage } from "./ui/LazyImage";
import { Play, Users } from "lucide-react";

interface AnimeGroupCardProps {
  group: AnimeGroup;
  onSelectGroup: (group: AnimeGroup) => void;
  onQuickPlay: (item: TorrentItem) => void;
}

export const AnimeGroupCard: React.FC<AnimeGroupCardProps> = ({
  group,
  onSelectGroup,
  onQuickPlay,
}) => {
  const { t } = useI18n();
  const anime = group.anime_details;
  const canonicalTitle = group.title || anime?.display_title || anime?.title_english || anime?.title_romaji;
  const bestRelease = group.best_release || group.releases[0];

  return (
    <div
      onClick={() => onSelectGroup(group)}
      className="group flex flex-col justify-between rounded-xl border border-zinc-800/80 bg-zinc-900/40 hover:bg-zinc-900/90 hover:border-zinc-700 transition-all duration-200 cursor-pointer overflow-hidden"
    >
      {/* Poster Box */}
      <div className="relative aspect-[16/10] w-full overflow-hidden bg-zinc-950">
        <LazyImage
          src={anime?.poster_image}
          alt={canonicalTitle || "Anime"}
          aspectRatio="aspect-[16/10]"
          imgClassName="group-hover:scale-105 transition-transform duration-300"
        />

        {/* Badges */}
        <div className="absolute top-2.5 inset-x-2.5 flex items-center justify-between pointer-events-none">
          <div className="flex items-center gap-1">
            <span className="rounded bg-zinc-950/80 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[10px] font-mono text-zinc-300">
              {group.release_count} {group.release_count === 1 ? "source" : "sources"}
            </span>
          </div>

          {anime?.average_score && anime.average_score > 0 ? (
            <span className="rounded bg-zinc-950/80 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[10px] font-mono text-zinc-200">
              ★ {anime.average_score.toFixed(1)}
            </span>
          ) : null}
        </div>

        {/* Quick Play Overlay */}
        {bestRelease && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onQuickPlay(bestRelease);
            }}
            title={t("Direct Stream")}
            className="absolute inset-0 m-auto flex h-11 w-11 items-center justify-center rounded-full bg-white text-zinc-950 opacity-0 group-hover:opacity-100 scale-90 group-hover:scale-100 transition-all duration-150 hover:bg-zinc-200 active:scale-95 shadow-md"
          >
            <Play className="h-4 w-4 fill-current ml-0.5" />
          </button>
        )}
      </div>

      {/* Content */}
      <div className="p-3.5 flex-1 flex flex-col justify-between gap-3">
        <div>
          <h3
            title={canonicalTitle}
            className="line-clamp-1 text-sm font-medium text-zinc-100 group-hover:text-white transition-colors"
          >
            {canonicalTitle}
          </h3>

          {/* Release groups list */}
          {group.release_groups.length > 0 ? (
            <p className="line-clamp-1 text-xs font-mono text-zinc-400 mt-1">
              {group.release_groups.join(", ")}
            </p>
          ) : (
            <p className="line-clamp-1 text-xs font-mono text-zinc-500 mt-1">
              {bestRelease ? bestRelease.title : t("Releases")}
            </p>
          )}

          {/* Genres */}
          {anime?.genres && anime.genres.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mt-2">
              {anime.genres.slice(0, 3).map((g) => (
                <span
                  key={g}
                  className="rounded bg-zinc-800/60 border border-zinc-800 px-1.5 py-0.5 text-[10px] text-zinc-300"
                >
                  {g}
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="pt-2 border-t border-zinc-800/60 flex items-center justify-between text-xs text-zinc-400 font-mono">
          <div className="flex items-center gap-1.5 text-zinc-300">
            <Users className="h-3 w-3 text-zinc-400" />
            <span>{group.max_seeders} {" "}{t("seeds")}</span>
          </div>
          <span className="text-zinc-400">{group.qualities.join(" ")}</span>
        </div>
      </div>
    </div>
  );
};
