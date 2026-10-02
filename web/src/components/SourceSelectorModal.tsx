"use client";
import { useI18n } from "@/lib/i18n";

import React, { useState, useMemo, useEffect } from "react";
import { AnimeGroup, TorrentItem } from "@/types/api";
import { LazyImage } from "./ui/LazyImage";
import {
  X,
  Play,
  Users,
  HardDrive,
  Search,
  Filter,
} from "lucide-react";

interface SourceSelectorModalProps {
  group: AnimeGroup | null;
  isOpen: boolean;
  onClose: () => void;
  onSelectRelease: (item: TorrentItem) => void;
}

export const SourceSelectorModal: React.FC<SourceSelectorModalProps> = ({
  group,
  isOpen,
  onClose,
  onSelectRelease,
}) => {
  const { t } = useI18n();
  const [selectedQuality, setSelectedQuality] = useState<string>("all");
  const [selectedGroup, setSelectedGroup] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [showFullDesc, setShowFullDesc] = useState<boolean>(false);
  const [prevGroupId, setPrevGroupId] = useState<string | null>(group?.id || null);

  if (group && group.id !== prevGroupId) {
    setPrevGroupId(group.id);
    setSelectedQuality("all");
    setSelectedGroup("all");
    setSearchQuery("");
    setShowFullDesc(false);
  }

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  const anime = group?.anime_details;
  const canonicalTitle =
    group?.title || anime?.display_title || anime?.title_english || anime?.title_romaji;

  const filteredReleases = useMemo(() => {
    if (!group) return [];

    return group.releases.filter((rel) => {
      if (selectedQuality !== "all") {
        if (!rel.title.toLowerCase().includes(selectedQuality.toLowerCase())) {
          return false;
        }
      }

      if (selectedGroup !== "all") {
        if (!rel.title.toLowerCase().includes(selectedGroup.toLowerCase())) {
          return false;
        }
      }

      if (searchQuery.trim() !== "") {
        if (!rel.title.toLowerCase().includes(searchQuery.toLowerCase())) {
          return false;
        }
      }

      return true;
    });
  }, [group, selectedQuality, selectedGroup, searchQuery]);

  if (!isOpen || !group) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6 bg-black/80 backdrop-blur-sm animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="relative flex flex-col w-full max-w-4xl max-h-[90vh] bg-zinc-950 border border-zinc-800 rounded-2xl shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="p-5 border-b border-zinc-800 bg-zinc-900/30 flex items-start gap-4">
          {anime?.poster_image && (
            <div className="hidden sm:block shrink-0 w-20 h-28 rounded-lg overflow-hidden border border-zinc-800 bg-zinc-900">
              <LazyImage
                src={anime.poster_image}
                alt={canonicalTitle || "Poster"}
                aspectRatio="aspect-[2/3]"
                priority
              />
            </div>
          )}

          <div className="flex-1 min-w-0">
            <div className="flex items-start justify-between gap-4">
              <div>
                <h2 className="text-lg font-semibold text-zinc-100 tracking-tight">
                  {canonicalTitle}
                </h2>
                {anime?.title_english && anime.title_english !== canonicalTitle && (
                  <p className="text-xs text-zinc-400 mt-0.5">{anime.title_english}</p>
                )}
              </div>

              <button
                onClick={onClose}
                className="p-1.5 rounded-lg text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* Badges */}
            <div className="flex flex-wrap items-center gap-1.5 mt-2.5">
              <span className="rounded bg-zinc-800 border border-zinc-700 px-2 py-0.5 text-[10px] font-mono text-zinc-300">
                {group.release_count} {t("Releases")}</span>
              {anime?.average_score && anime.average_score > 0 ? (
                <span className="rounded bg-zinc-800 border border-zinc-700 px-2 py-0.5 text-[10px] font-mono text-zinc-200">
                  ★ {anime.average_score.toFixed(1)}
                </span>
              ) : null}
              {anime?.genres?.slice(0, 3).map((genre) => (
                <span
                  key={genre}
                  className="rounded bg-zinc-900 border border-zinc-800 px-2 py-0.5 text-[10px] text-zinc-400"
                >
                  {genre}
                </span>
              ))}
            </div>

            {anime?.description && (
              <div className="mt-2 text-xs text-zinc-400 leading-relaxed max-w-2xl">
                <p className={showFullDesc ? "" : "line-clamp-2"}>
                  {anime.description.replace(/<[^>]*>/g, "")}
                </p>
                {anime.description.length > 140 && (
                  <button
                    onClick={() => setShowFullDesc(!showFullDesc)}
                    className="text-[11px] text-zinc-300 hover:text-white underline mt-0.5 cursor-pointer"
                  >
                    {showFullDesc ? t("Show less") : t("Read more")}
                  </button>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Filters */}
        <div className="p-3 bg-zinc-900/20 border-b border-zinc-800 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Filter className="h-3.5 w-3.5 text-zinc-500" />
            <select
              value={selectedQuality}
              onChange={(e) => setSelectedQuality(e.target.value)}
              className="bg-zinc-900 border border-zinc-800 rounded px-2.5 py-1 text-xs text-zinc-200 focus:outline-none focus:border-zinc-500"
            >
              <option value="all">{t("All Qualities")}</option>
              {group.qualities.map((q) => (
                <option key={q} value={q}>
                  {q}
                </option>
              ))}
            </select>

            {group.release_groups.length > 0 && (
              <select
                value={selectedGroup}
                onChange={(e) => setSelectedGroup(e.target.value)}
                className="bg-zinc-900 border border-zinc-800 rounded px-2.5 py-1 text-xs text-zinc-200 focus:outline-none focus:border-zinc-500"
              >
                <option value="all">{t("All Groups")}</option>
                {group.release_groups.map((g) => (
                  <option key={g} value={g}>
                    {g}
                  </option>
                ))}
              </select>
            )}
          </div>

          <div className="relative flex-1 sm:w-48 max-w-xs">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500" />
            <input
              type="text"
              placeholder={t("Search release...")}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-zinc-900 border border-zinc-800 rounded pl-8 pr-2.5 py-1 text-xs text-zinc-200 placeholder-zinc-500 focus:outline-none focus:border-zinc-500"
            />
          </div>
        </div>

        {/* Releases List */}
        <div className="flex-1 overflow-y-auto p-4 space-y-2 max-h-[50vh]">
          {filteredReleases.map((item, idx) => (
            <div
              key={item.info_hash || idx}
              className="group flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3 rounded-lg border border-zinc-800/80 bg-zinc-900/40 hover:bg-zinc-900/90 hover:border-zinc-700 transition-all"
            >
              <div className="flex-1 min-w-0">
                <p
                  title={item.title}
                  className="text-xs font-mono text-zinc-200 group-hover:text-white transition-colors line-clamp-2 leading-relaxed"
                >
                  {item.title}
                </p>

                <div className="flex flex-wrap items-center gap-3 mt-1.5 text-[11px] text-zinc-400 font-mono">
                  <div className="flex items-center gap-1 text-zinc-300">
                    <Users className="h-3 w-3 text-zinc-400" />
                    <span>{item.seeders} {" "}{t("seeds")}</span>
                  </div>

                  <div className="flex items-center gap-1 text-zinc-400">
                    <HardDrive className="h-3 w-3 text-zinc-500" />
                    <span>{item.size_display}</span>
                  </div>
                </div>
              </div>

              <div className="shrink-0 flex items-center justify-end">
                <button
                  onClick={() => onSelectRelease(item)}
                  className="flex items-center gap-1.5 rounded-lg bg-zinc-100 hover:bg-white text-zinc-950 px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer"
                >
                  <Play className="h-3.5 w-3.5 fill-current" />
                  <span>{t("Stream")}</span>
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
