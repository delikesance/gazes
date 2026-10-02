"use client";
import { useI18n } from "@/lib/i18n";

import React from "react";
import { TorrentItem } from "@/types/api";
import { LazyImage } from "./ui/LazyImage";
import { Play, Users } from "lucide-react";

interface TorrentCardProps {
  item: TorrentItem;
  onPlay: (item: TorrentItem) => void;
}

export const TorrentCard: React.FC<TorrentCardProps> = ({ item, onPlay }) => {
  const { t } = useI18n();
  const is1080p = item.title.includes("1080p");
  const is720p = item.title.includes("720p");
  const is4k = item.title.includes("2160p") || item.title.includes("4K");

  const formattedDate = new Date(item.publish_date).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  });

  const anime = item.anime_details;
  const canonicalTitle = anime?.display_title || anime?.title_english || anime?.title_romaji;

  return (
    <div className="group flex flex-col justify-between rounded-xl border border-zinc-800/80 bg-zinc-900/40 hover:bg-zinc-900/90 hover:border-zinc-700 transition-all duration-200 overflow-hidden">
      {/* Media Box */}
      <div className="relative aspect-[16/10] w-full overflow-hidden bg-zinc-950">
        <LazyImage
          src={anime?.poster_image}
          alt={canonicalTitle || item.title}
          aspectRatio="aspect-[16/10]"
          imgClassName="group-hover:scale-105 transition-transform duration-300"
        />

        {/* Top Badges */}
        <div className="absolute top-2.5 inset-x-2.5 flex items-center justify-between pointer-events-none">
          <div className="flex items-center gap-1">
            {is4k && (
              <span className="rounded bg-zinc-950/80 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[9px] font-mono text-zinc-200">
                4K
              </span>
            )}
            {is1080p && (
              <span className="rounded bg-zinc-950/80 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[9px] font-mono text-zinc-200">
                1080p
              </span>
            )}
            {is720p && (
              <span className="rounded bg-zinc-950/80 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[9px] font-mono text-zinc-200">
                720p
              </span>
            )}
          </div>

          {anime?.average_score && anime.average_score > 0 ? (
            <span className="rounded bg-zinc-950/80 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[10px] font-mono text-zinc-200">
              ★ {anime.average_score.toFixed(1)}
            </span>
          ) : null}
        </div>

        {/* Quick Play Button */}
        <button
          onClick={() => onPlay(item)}
          title={t("Direct Stream")}
          className="absolute inset-0 m-auto flex h-11 w-11 items-center justify-center rounded-full bg-white text-zinc-950 opacity-0 group-hover:opacity-100 scale-90 group-hover:scale-100 transition-all duration-150 hover:bg-zinc-200 active:scale-95 shadow-md"
        >
          <Play className="h-4 w-4 fill-current ml-0.5" />
        </button>
      </div>

      {/* Content */}
      <div className="p-3.5 flex-1 flex flex-col justify-between gap-3">
        <div>
          {canonicalTitle ? (
            <div>
              <h3
                title={canonicalTitle}
                className="line-clamp-1 text-sm font-medium text-zinc-100 group-hover:text-white transition-colors"
              >
                {canonicalTitle}
              </h3>
              <p
                title={item.title}
                className="line-clamp-1 text-xs font-mono text-zinc-400 mt-0.5"
              >
                {item.title}
              </p>
            </div>
          ) : (
            <h3
              title={item.title}
              className="line-clamp-2 text-xs font-medium text-zinc-200 group-hover:text-white transition-colors"
            >
              {item.title}
            </h3>
          )}
        </div>

        {/* Stats & Actions */}
        <div className="pt-2 border-t border-zinc-800/60">
          <div className="flex items-center justify-between text-xs text-zinc-400 font-mono mb-2">
            <div className="flex items-center gap-1 text-zinc-300">
              <Users className="h-3 w-3 text-zinc-400" />
              <span>{item.seeders} {" "}{t("seeds")}</span>
            </div>
            <div className="flex items-center gap-2 text-zinc-400 text-[11px]">
              <span>{item.size_display}</span>
              <span>•</span>
              <span>{formattedDate}</span>
            </div>
          </div>

          <button
            onClick={() => onPlay(item)}
            className="w-full flex items-center justify-center gap-1.5 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-xs font-medium text-zinc-200 hover:text-white transition-colors cursor-pointer"
          >
            <Play className="h-3 w-3 fill-current" />
            <span>{t("Stream")}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
