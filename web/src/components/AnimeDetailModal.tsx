"use client";
import { useI18n } from "@/lib/i18n";

import React, { useState, useEffect } from "react";
import { AnimeCatalogItem, EpisodeInfo, EpisodeSource, EpisodeSourcesResponse, AnimeRelation } from "@/types/api";
import { getAnimeCatalogDetail, getEpisodeSources } from "@/lib/api";
import { EpisodeList } from "@/components/EpisodeList";
import { EpisodeSourceSelectorModal } from "@/components/EpisodeSourceSelectorModal";
import { LazyImage } from "./ui/LazyImage";
import { Button } from "./ui/Button";
import { X, Play, Loader2, Film, Layers } from "lucide-react";

interface AnimeDetailModalProps {
  animeId: number | null;
  isOpen: boolean;
  onClose: () => void;
  onPlayEpisodeSource: (source: EpisodeSource, anime: AnimeCatalogItem, episodeNum: number) => void;
}

export const AnimeDetailModal: React.FC<AnimeDetailModalProps> = ({
  animeId,
  isOpen,
  onClose,
  onPlayEpisodeSource,
}) => {
  const { t } = useI18n();
  const [activeAnimeId, setActiveAnimeId] = useState<number | null>(animeId);
  const [anime, setAnime] = useState<AnimeCatalogItem | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [showFullDesc, setShowFullDesc] = useState<boolean>(false);
  const [resolvingEpNumber, setResolvingEpNumber] = useState<number | null>(null);

  // Sync activeAnimeId when animeId prop changes
  const [prevPropId, setPrevPropId] = useState<number | null>(animeId);
  if (animeId !== prevPropId) {
    setPrevPropId(animeId);
    setActiveAnimeId(animeId);
    setLoading(true);
    setError(null);
    setShowFullDesc(false);
  }

  // Source selector modal state
  const [selectedEpForSources, setSelectedEpForSources] = useState<EpisodeInfo | null>(null);
  const [sourcesData, setSourcesData] = useState<EpisodeSourcesResponse | null>(null);
  const [loadingSources, setLoadingSources] = useState<boolean>(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen && !selectedEpForSources) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, selectedEpForSources, onClose]);

  useEffect(() => {
    if (!activeAnimeId || !isOpen) return;

    let ignore = false;
    getAnimeCatalogDetail(activeAnimeId)
      .then((data) => {
        if (!ignore) setAnime(data);
      })
      .catch((err) => {
        if (!ignore) setError(err.message || "Failed to load anime");
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });

    return () => {
      ignore = true;
    };
  }, [activeAnimeId, isOpen]);

  // Handle Quick Play Episode (Resolves sources on-demand upon clicking)
  const handlePlayEpisode = async (ep: EpisodeInfo) => {
    if (!anime) return;
    setResolvingEpNumber(ep.episode_number);
    try {
      const res = await getEpisodeSources(anime.id, ep.episode_number, anime.display_title);
      if (res && res.sources && res.sources.length > 0) {
        onPlayEpisodeSource(res.sources[0], anime, ep.episode_number);
      } else {
        setSelectedEpForSources(ep);
        setSourcesData(res);
      }
    } catch {
      setSelectedEpForSources(ep);
    } finally {
      setResolvingEpNumber(null);
    }
  };

  // Handle Open Sources Modal (Resolves all sources for inspecting)
  const handleOpenSources = async (ep: EpisodeInfo) => {
    if (!anime) return;
    setSelectedEpForSources(ep);
    setLoadingSources(true);
    try {
      const res = await getEpisodeSources(anime.id, ep.episode_number, anime.display_title);
      setSourcesData(res);
    } catch (err) {
      console.error("Failed to load sources:", err);
    } finally {
      setLoadingSources(false);
    }
  };

  // Switch to a related season or entry
  const handleSelectRelation = (relation: AnimeRelation) => {
    setActiveAnimeId(relation.id);
    setLoading(true);
    setError(null);
    setShowFullDesc(false);
  };

  if (!isOpen) return null;

  // Filter relevant anime relations (seasons, prequels, sequels, movies, side stories)
  const seasonRelations = anime?.relations?.filter((r) =>
    ["PREQUEL", "SEQUEL", "SIDE_STORY", "ALTERNATIVE", "PARENT", "SPIN_OFF"].includes(r.relation_type)
  ) || [];

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center p-3 sm:p-6 bg-black/80 backdrop-blur-sm animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="relative flex flex-col w-full max-w-5xl max-h-[92vh] bg-zinc-950 border border-zinc-800 rounded-2xl shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {loading ? (
          <div className="py-32 flex flex-col items-center justify-center space-y-3">
            <Loader2 className="h-8 w-8 text-zinc-400 animate-spin" />
            <p className="text-xs text-zinc-400">{t("Loading anime details & seasons...")}</p>
          </div>
        ) : error || !anime ? (
          <div className="p-12 text-center">
            <Film className="h-8 w-8 text-zinc-600 mx-auto mb-3" />
            <p className="text-xs text-zinc-300">{error || "Anime not found"}</p>
            <Button variant="secondary" size="sm" onClick={onClose} className="mt-4">
              {t("Close")}</Button>
          </div>
        ) : (
          <>
            {/* Header Hero */}
            <div className="relative border-b border-zinc-800 p-5 sm:p-6 shrink-0 bg-zinc-900/30">
              <div className="flex flex-col sm:flex-row gap-5 items-start">
                {/* Poster Image */}
                {anime.poster_image && (
                  <div className="hidden sm:block shrink-0 w-28 h-40 rounded-lg overflow-hidden border border-zinc-800 bg-zinc-900">
                    <LazyImage
                      src={anime.poster_image}
                      alt={anime.franchise_title || anime.display_title}
                      aspectRatio="aspect-[2/3]"
                      priority
                    />
                  </div>
                )}

                {/* Details */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <h2 className="text-lg sm:text-xl font-semibold text-zinc-100 tracking-tight">
                        {anime.franchise_title || anime.display_title}
                      </h2>
                      {anime.display_title !== (anime.franchise_title || anime.display_title) ? (
                        <p className="text-xs text-zinc-400 mt-0.5 font-medium">{anime.display_title}</p>
                      ) : anime.title_romaji && anime.title_romaji !== anime.display_title ? (
                        <p className="text-xs text-zinc-400 mt-0.5">{anime.title_romaji}</p>
                      ) : null}
                    </div>

                    <button
                      onClick={onClose}
                      className="p-1.5 rounded-lg text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
                    >
                      <X className="h-5 w-5" />
                    </button>
                  </div>

                  {/* Badges */}
                  <div className="flex flex-wrap items-center gap-1.5 mt-3">
                    {anime.average_score && anime.average_score > 0 ? (
                      <span className="rounded bg-zinc-800 border border-zinc-700 px-2 py-0.5 text-[10px] font-mono text-zinc-200">
                        ★ {anime.average_score.toFixed(1)}
                      </span>
                    ) : null}

                    {anime.episodes ? (
                      <span className="rounded bg-zinc-800 border border-zinc-700 px-2 py-0.5 text-[10px] font-mono text-zinc-300">
                        {anime.episodes} {t("Episodes")}</span>
                    ) : null}

                    {anime.genres?.map((genre) => (
                      <span
                        key={genre}
                        className="rounded bg-zinc-900 border border-zinc-800 px-2 py-0.5 text-[10px] text-zinc-400"
                      >
                        {genre}
                      </span>
                    ))}

                    {anime.season_year ? (
                      <span className="rounded bg-zinc-900 border border-zinc-800 px-2 py-0.5 text-[10px] text-zinc-400 font-mono">
                        {anime.season_year}
                      </span>
                    ) : null}
                  </div>

                  {/* Synopsis */}
                  {anime.description && (
                    <div className="mt-3 text-xs text-zinc-400 leading-relaxed max-w-3xl">
                      <p className={showFullDesc ? "" : "line-clamp-2"}>
                        {anime.description}
                      </p>
                      {anime.description.length > 150 && (
                        <button
                          onClick={() => setShowFullDesc(!showFullDesc)}
                          className="text-[11px] text-zinc-300 hover:text-white underline mt-1 cursor-pointer"
                        >
                          {showFullDesc ? t("Show less") : t("Read more")}
                        </button>
                      )}
                    </div>
                  )}

                  {/* Play Ep 1 Button */}
                  {anime.episode_list && anime.episode_list.length > 0 && (
                    <div className="mt-4">
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => handlePlayEpisode(anime.episode_list![0])}
                        disabled={resolvingEpNumber === 1}
                      >
                        {resolvingEpNumber === 1 ? (
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <Play className="h-3.5 w-3.5 fill-current" />
                        )}
                        <span>{t("Play Episode 1")}</span>
                      </Button>
                    </div>
                  )}
                </div>
              </div>
            </div>

            {/* Seasons & Relations Selector Bar */}
            {anime.seasons && anime.seasons.length > 1 ? (
              <div className="px-5 py-3 border-b border-zinc-800/80 bg-zinc-900/40">
                <div className="flex items-center gap-2 overflow-x-auto pb-1 text-xs no-scrollbar">
                  <span className="text-zinc-500 font-mono flex items-center gap-1 shrink-0 mr-1 text-[11px]">
                    <Layers className="h-3.5 w-3.5" /> {t("SAISONS:")}</span>
                  {anime.seasons.map((season) => {
                    const isCurrent = season.id === anime.id;
                    return (
                      <button
                        key={season.id}
                        onClick={() => {
                          if (!isCurrent) {
                            setActiveAnimeId(season.id);
                            setLoading(true);
                            setError(null);
                            setShowFullDesc(false);
                          }
                        }}
                        className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-all whitespace-nowrap cursor-pointer flex items-center gap-1.5 ${
                          isCurrent
                            ? "bg-white text-zinc-950 shadow-sm"
                            : "bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 text-zinc-300 hover:text-white"
                        }`}
                      >
                        <span>{season.season_name || season.title}</span>
                        {season.episodes ? (
                          <span
                            className={`text-[10px] font-mono px-1 py-0.2 rounded ${
                              isCurrent ? "bg-zinc-200 text-zinc-800" : "bg-zinc-800 text-zinc-400"
                            }`}
                          >
                            {season.episodes} {t("ep")}</span>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              </div>
            ) : seasonRelations.length > 0 ? (
              <div className="px-5 py-3 border-b border-zinc-800/80 bg-zinc-900/20">
                <div className="flex items-center gap-2 overflow-x-auto pb-1 text-xs no-scrollbar">
                  <span className="text-zinc-500 font-mono flex items-center gap-1 shrink-0 mr-1 text-[11px]">
                    <Layers className="h-3.5 w-3.5" /> {t("SAISONS:")}</span>
                  <span className="rounded-lg bg-white text-zinc-950 font-medium px-3 py-1.5 text-xs whitespace-nowrap shadow-sm">
                    {t("Saison 1 (")}{anime.episodes ? t("{count} ép.", {count:anime.episodes}) : t("Principale")})
                  </span>
                  {seasonRelations.map((rel) => (
                    <button
                      key={rel.id}
                      onClick={() => handleSelectRelation(rel)}
                      title={rel.display_title}
                      className="rounded-lg bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 text-zinc-300 hover:text-white px-3 py-1.5 text-xs transition-colors whitespace-nowrap cursor-pointer flex items-center gap-1.5"
                    >
                      <span>{rel.display_title}</span>
                      <span className="text-[10px] font-mono text-zinc-500">
                        ({rel.relation_type.toLowerCase().replace("_", " ")})
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}

            {/* Episode List Section */}
            <div className="flex-1 overflow-y-auto p-5">
              {anime.episode_list && anime.episode_list.length > 0 ? (
                <EpisodeList
                  anime={anime}
                  episodes={anime.episode_list}
                  loadingEpNumber={resolvingEpNumber}
                  onPlayEpisode={handlePlayEpisode}
                  onOpenSources={handleOpenSources}
                />
              ) : (
                <div className="py-12 text-center text-zinc-500">
                  <Film className="h-8 w-8 mx-auto mb-2 opacity-30 stroke-1" />
                  <p className="text-xs">{t("No episodes listed for this anime.")}</p>
                </div>
              )}
            </div>
          </>
        )}
      </div>

      {/* Episode Sources Modal */}
      {selectedEpForSources && (
        <EpisodeSourceSelectorModal
          animeTitle={anime?.display_title || ""}
          episodeNumber={selectedEpForSources.episode_number}
          sourcesData={sourcesData}
          isLoading={loadingSources}
          isOpen={Boolean(selectedEpForSources)}
          onClose={() => {
            setSelectedEpForSources(null);
            setSourcesData(null);
          }}
          onSelectSource={(src) => {
            if (anime) {
              onPlayEpisodeSource(src, anime, selectedEpForSources.episode_number);
              setSelectedEpForSources(null);
              setSourcesData(null);
            }
          }}
        />
      )}
    </div>
  );
};
