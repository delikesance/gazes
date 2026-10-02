"use client";
import { useI18n } from "@/lib/i18n";

import React, { useState, useMemo } from "react";
import { EpisodeInfo, AnimeCatalogItem } from "@/types/api";
import { LazyImage } from "./ui/LazyImage";
import { Play, Layers, Film, Search, ChevronLeft, ChevronRight, Loader2 } from "lucide-react";

interface EpisodeListProps {
  anime: AnimeCatalogItem;
  episodes: EpisodeInfo[];
  currentPlayingEp?: number;
  loadingEpNumber?: number | null;
  onPlayEpisode: (ep: EpisodeInfo) => void;
  onOpenSources: (ep: EpisodeInfo) => void;
}

const ITEMS_PER_PAGE = 24;

export const EpisodeList: React.FC<EpisodeListProps> = ({
  anime,
  episodes,
  currentPlayingEp,
  loadingEpNumber,
  onPlayEpisode,
  onOpenSources,
}) => {
  const { t } = useI18n();
  const [filterQuery, setFilterQuery] = useState("");
  const [page, setPage] = useState(1);

  const filteredEpisodes = useMemo(() => {
    if (!filterQuery.trim()) return episodes;
    const q = filterQuery.toLowerCase();
    return episodes.filter(
      (ep) =>
        ep.title.toLowerCase().includes(q) ||
        `episode ${ep.episode_number}`.includes(q) ||
        `${ep.episode_number}` === q
    );
  }, [episodes, filterQuery]);

  const totalPages = Math.ceil(filteredEpisodes.length / ITEMS_PER_PAGE);

  const paginatedEpisodes = useMemo(() => {
    const start = (page - 1) * ITEMS_PER_PAGE;
    return filteredEpisodes.slice(start, start + ITEMS_PER_PAGE);
  }, [filteredEpisodes, page]);

  // Generate range chunks for fast jumping (e.g. 1-50, 51-100, etc.)
  const rangeChunks = useMemo(() => {
    if (episodes.length <= ITEMS_PER_PAGE) return [];
    const chunks = [];
    const chunkSize = 50;
    for (let i = 0; i < episodes.length; i += chunkSize) {
      const startEp = i + 1;
      const endEp = Math.min(i + chunkSize, episodes.length);
      chunks.push({ label: `${startEp}-${endEp}`, startIdx: i });
    }
    return chunks;
  }, [episodes.length]);

  return (
    <div className="space-y-4">
      {/* Episodes Toolbar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-zinc-900/40 border border-zinc-800/80 p-3 rounded-xl">
        <div className="flex items-center gap-2">
          <span className="text-xs sm:text-sm font-medium text-zinc-200">{t("Episodes")}</span>
          <span className="rounded bg-zinc-800 border border-zinc-700 px-2 py-0.5 text-xs font-mono text-zinc-300">
            {filteredEpisodes.length}
          </span>
        </div>

        {/* Filter input */}
        <div className="relative w-full sm:w-60">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500" />
          <input
            type="text"
            placeholder={t("Search episode or #...")}
            value={filterQuery}
            onChange={(e) => {
              setFilterQuery(e.target.value);
              setPage(1);
            }}
            className="w-full bg-zinc-900/80 border border-zinc-800 rounded-lg pl-8 pr-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-500 focus:outline-none focus:border-zinc-500 font-mono"
          />
        </div>
      </div>

      {/* Range Chunks for Long Series */}
      {rangeChunks.length > 1 && !filterQuery && (
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
          <span className="text-zinc-500 font-mono shrink-0 mr-1">{t("Jump to:")}</span>
          {rangeChunks.map((chunk) => {
            const chunkPage = Math.floor(chunk.startIdx / ITEMS_PER_PAGE) + 1;
            const isSelected = Math.abs(page - chunkPage) <= 1;
            return (
              <button
                key={chunk.label}
                onClick={() => setPage(chunkPage)}
                className={`rounded px-2 py-0.5 font-mono text-[11px] transition-colors whitespace-nowrap cursor-pointer ${
                  isSelected
                    ? "bg-zinc-100 text-zinc-950 font-semibold"
                    : "bg-zinc-900 text-zinc-400 hover:text-zinc-200 border border-zinc-800"
                }`}
              >
                {chunk.label}
              </button>
            );
          })}
        </div>
      )}

      {/* Grid of Episodes */}
      {filteredEpisodes.length === 0 ? (
        <div className="py-12 text-center text-zinc-500">
          <Film className="h-8 w-8 mx-auto mb-2 opacity-30 stroke-1" />
          <p className="text-xs">{t("No episode matched your search query.")}</p>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3 max-h-[50vh] overflow-y-auto pr-1">
            {paginatedEpisodes.map((ep) => {
              const isPlaying = currentPlayingEp === ep.episode_number;
              const isLoading = loadingEpNumber === ep.episode_number;

              return (
                <div
                  key={ep.episode_number}
                  onClick={() => onPlayEpisode(ep)}
                  className={`group flex flex-col justify-between rounded-xl border bg-zinc-900/40 hover:bg-zinc-900/80 transition-all duration-150 overflow-hidden cursor-pointer ${
                    isPlaying
                      ? "border-white/80 ring-1 ring-white/40"
                      : "border-zinc-800/80 hover:border-zinc-700"
                  }`}
                >
                  {/* Thumbnail Box */}
                  <div className="relative aspect-video w-full overflow-hidden bg-zinc-950">
                    <LazyImage
                      src={ep.thumbnail || anime.poster_image}
                      alt={ep.title}
                      aspectRatio="aspect-video"
                      imgClassName="group-hover:scale-105 transition-transform duration-300"
                    />

                    {/* Episode Number Badge */}
                    <div className="absolute top-2 left-2 pointer-events-none">
                      <span className="rounded bg-zinc-950/85 backdrop-blur-sm border border-zinc-800 px-1.5 py-0.5 text-[10px] font-mono text-zinc-200">
                        EP {ep.episode_number}
                      </span>
                    </div>

                    {/* Quick Play Button */}
                    <div className="absolute inset-0 m-auto flex h-9 w-9 items-center justify-center rounded-full bg-white text-zinc-950 opacity-0 group-hover:opacity-100 scale-90 group-hover:scale-100 transition-all duration-150 shadow-md">
                      {isLoading ? (
                        <Loader2 className="h-4 w-4 animate-spin text-zinc-950" />
                      ) : (
                        <Play className="h-3.5 w-3.5 fill-current ml-0.5" />
                      )}
                    </div>
                  </div>

                  {/* Info & Actions */}
                  <div className="p-3 flex-1 flex flex-col justify-between gap-2.5">
                    <div>
                      <h4
                        title={ep.title}
                        className="line-clamp-2 text-xs font-medium text-zinc-200 group-hover:text-white transition-colors leading-snug"
                      >
                        {ep.title}
                      </h4>
                    </div>

                    <div className="pt-2 border-t border-zinc-800/60 flex items-center gap-2">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onPlayEpisode(ep);
                        }}
                        disabled={isLoading}
                        className="flex-1 flex items-center justify-center gap-1.5 py-1.5 rounded-lg bg-zinc-100 hover:bg-white text-zinc-950 text-xs font-medium transition-colors cursor-pointer disabled:opacity-50"
                      >
                        {isLoading ? (
                          <>
                            <Loader2 className="h-3 w-3 animate-spin text-zinc-950" />
                            <span>{t("Finding sources...")}</span>
                          </>
                        ) : (
                          <>
                            <Play className="h-3 w-3 fill-current" />
                            <span>{t("Play")}</span>
                          </>
                        )}
                      </button>

                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onOpenSources(ep);
                        }}
                        title={t("Browse all available sources / resolutions")}
                        className="flex items-center justify-center rounded-lg bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 px-2 py-1.5 text-xs text-zinc-300 hover:text-white transition-colors cursor-pointer"
                      >
                        <Layers className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>

          {/* Pagination Controls when there are multiple pages */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t border-zinc-800/80 pt-3 text-xs text-zinc-400 font-mono">
              <span>
                {t("Page")} {page} {" "}{t("of")}{" "}{totalPages}
              </span>
              <div className="flex items-center gap-1.5">
                <button
                  disabled={page <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  className="p-1.5 rounded-md border border-zinc-800 bg-zinc-900 text-zinc-300 disabled:opacity-30 hover:bg-zinc-800 cursor-pointer"
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                </button>
                <button
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  className="p-1.5 rounded-md border border-zinc-800 bg-zinc-900 text-zinc-300 disabled:opacity-30 hover:bg-zinc-800 cursor-pointer"
                >
                  <ChevronRight className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
};
