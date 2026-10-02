"use client";
import { useI18n } from "@/lib/i18n";

import React, { useState, useMemo, useEffect, useRef } from "react";
import { EpisodeSource, EpisodeSourcesResponse } from "@/types/api";
import {
  X,
  Play,
  Users,
  HardDrive,
  Search,
  Filter,
  Film,
  Loader2,
} from "lucide-react";

interface EpisodeSourceSelectorModalProps {
  animeTitle: string;
  episodeNumber: number;
  sourcesData: EpisodeSourcesResponse | null;
  currentSourceHash?: string;
  isLoading: boolean;
  isOpen: boolean;
  onClose: () => void;
  onSelectSource: (source: EpisodeSource) => void;
}

export const EpisodeSourceSelectorModal: React.FC<EpisodeSourceSelectorModalProps> = ({
  animeTitle,
  episodeNumber,
  sourcesData,
  currentSourceHash,
  isLoading,
  isOpen,
  onClose,
  onSelectSource,
}) => {
  const { t } = useI18n();
  const dialogRef = useRef<HTMLDivElement>(null);
  const [selectedLang, setSelectedLang] = useState<string>("all");
  const [selectedQuality, setSelectedQuality] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [prevKey, setPrevKey] = useState<string>(`${animeTitle}-${episodeNumber}`);

  const currentKey = `${animeTitle}-${episodeNumber}`;
  if (currentKey !== prevKey) {
    setPrevKey(currentKey);
    setSelectedLang("all");
    setSelectedQuality("all");
    setSearchQuery("");
  }

  useEffect(() => {
    if (!isOpen) return;
    const previousFocus = document.activeElement as HTMLElement | null;
    dialogRef.current?.querySelector<HTMLButtonElement>('button')?.focus();
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      } else if (e.key === "Tab") {
        const controls = dialogRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input, select');
        const first = controls?.[0], last = controls?.[controls.length - 1];
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus(); }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus(); }
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => { window.removeEventListener("keydown", handleKeyDown); if (previousFocus?.isConnected) previousFocus.focus(); };
  }, [isOpen, onClose]);

  const sources = useMemo(() => sourcesData?.sources || [], [sourcesData]);

  const filteredSources = useMemo(() => {
    return sources.filter((src) => {
      // Language filter
      if (selectedLang === "french" && !src.is_french) return false;
      if (selectedLang === "vostfr" && src.language_tag !== "VOSTFR" && !src.language_flags?.includes("VOSTFR")) return false;
      if (selectedLang === "vf" && src.language_tag !== "VF") return false;
      if (selectedLang === "multi" && src.language_tag !== "MULTI") return false;
      if (selectedLang === "vosten" && src.language_tag !== "VOSTEN") return false;

      // Quality filter
      if (selectedQuality !== "all") {
        if (!src.quality.toLowerCase().includes(selectedQuality.toLowerCase())) {
          return false;
        }
      }

      // Search query
      if (searchQuery.trim() !== "") {
        const q = searchQuery.toLowerCase();
        if (!src.title.toLowerCase().includes(q) && !src.release_group.toLowerCase().includes(q)) {
          return false;
        }
      }

      return true;
    });
  }, [sources, selectedLang, selectedQuality, searchQuery]);

  if (!isOpen) return null;

  const getLangBadge = (src: EpisodeSource) => {
    return (
      <span className="inline-flex items-center rounded-full bg-zinc-800 border border-zinc-700 px-1.5 py-0.5 text-[10px] font-mono text-zinc-300">
        {src.language_tag || "OTHER"}
      </span>
    );
  };

  return (
    <div
      ref={dialogRef}
      role="dialog" aria-modal="true" aria-label={t("Changer de source pour cet épisode")}
      className="fixed inset-0 z-[60] flex items-center justify-center p-4 sm:p-6 bg-black/80 backdrop-blur-sm animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="relative flex flex-col w-full max-w-4xl max-h-[88vh] bg-zinc-950 border border-zinc-800 rounded-[28px] shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="p-4 sm:p-5 bg-zinc-900/40 border-b border-zinc-800 flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <span className="rounded-full bg-zinc-800 border border-zinc-700 px-2 py-0.5 text-xs font-mono text-zinc-200">
                EP {episodeNumber}
              </span>
              <h2 className="text-base sm:text-lg font-semibold text-zinc-100 tracking-tight">
                {animeTitle}
              </h2>
            </div>
            <p className="text-xs text-zinc-400 mt-1 font-mono">
              {sources.length} {t("sources available")}{sourcesData && sourcesData.french_sources > 0 && (
                <span className="text-zinc-300 ml-1.5">
                  ({sourcesData.french_sources} {t("with French sub/dub)")}</span>
              )}
            </p>
          </div>

          <button
            onClick={onClose}
            aria-label={t("Fermer")}
            className="p-1.5 rounded-full text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {sourcesData?.partial && <p className="px-4 py-2 text-xs text-amber-200">{t("Recherche partielle : certaines sources peuvent manquer.")}</p>}
        {/* Filters Bar */}
        <div className="p-3 bg-zinc-900/20 border-b border-zinc-800 flex flex-wrap items-center justify-between gap-3">
          {/* Language filter buttons */}
          <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
            <span className="text-xs text-zinc-400 mr-1 flex items-center gap-1">
              <Filter className="h-3 w-3" /> {t("Lang:")}</span>
            <button
              onClick={() => setSelectedLang("all")}
              className={`rounded-full px-2.5 py-1 text-xs font-medium transition-all cursor-pointer ${
                selectedLang === "all"
                  ? "bg-zinc-100 text-zinc-950"
                  : "bg-zinc-900 text-zinc-400 hover:text-zinc-200 border border-zinc-800"
              }`}
            >
              {t("All (")}{sources.length})
            </button>
            <button
              onClick={() => setSelectedLang("french")}
              className={`rounded-full px-2.5 py-1 text-xs font-medium transition-all cursor-pointer ${
                selectedLang === "french"
                  ? "bg-zinc-100 text-zinc-950"
                  : "bg-zinc-900 text-zinc-400 hover:text-zinc-200 border border-zinc-800"
              }`}
            >
              {t("FR (")}{sourcesData?.french_sources || 0})
            </button>
            <button
              onClick={() => setSelectedLang("vostfr")}
              className={`rounded-full px-2.5 py-1 text-xs font-medium transition-all cursor-pointer ${
                selectedLang === "vostfr"
                  ? "bg-zinc-100 text-zinc-950"
                  : "bg-zinc-900 text-zinc-400 hover:text-zinc-200 border border-zinc-800"
              }`}
            >
              VOSTFR
            </button>
            <button
              onClick={() => setSelectedLang("vf")}
              className={`rounded-full px-2.5 py-1 text-xs font-medium transition-all cursor-pointer ${
                selectedLang === "vf"
                  ? "bg-zinc-100 text-zinc-950"
                  : "bg-zinc-900 text-zinc-400 hover:text-zinc-200 border border-zinc-800"
              }`}
            >
              VF
            </button>
          </div>

          {/* Quality & Search */}
          <div className="flex items-center gap-2 flex-1 sm:flex-initial min-w-[200px]">
            <select
              value={selectedQuality}
              onChange={(e) => setSelectedQuality(e.target.value)}
              className="bg-zinc-900 border border-zinc-800 rounded-full px-2.5 py-1 text-xs text-zinc-200 focus:outline-none focus:border-zinc-500 font-mono"
            >
              <option value="all">{t("All Qualities")}</option>
              <option value="1080p">1080p</option>
              <option value="720p">720p</option>
              <option value="4K">4K UHD</option>
            </select>

            <div className="relative flex-1 sm:w-44">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500" />
              <input
                type="text"
                placeholder={t("Filter releases...")}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-zinc-900 border border-zinc-800 rounded-full pl-8 pr-2.5 py-1 text-xs text-zinc-200 placeholder-zinc-500 focus:outline-none focus:border-zinc-500"
              />
            </div>
          </div>
        </div>

        {/* Content list */}
        <div className="flex-1 overflow-y-auto p-4 space-y-2 max-h-[50vh]">
          {isLoading ? (
            <div className="py-16 flex flex-col items-center justify-center space-y-3">
              <Loader2 className="h-8 w-8 text-zinc-400 animate-spin" />
              <p className="text-xs text-zinc-400">
                {t("Searching sources for episode")}{episodeNumber}...
              </p>
            </div>
          ) : filteredSources.length === 0 ? (
            <div className="py-16 text-center text-zinc-500 max-w-md mx-auto px-4">
              <Film className="h-8 w-8 mx-auto mb-2 opacity-30 stroke-1 text-zinc-400" />
              <p className="text-xs text-zinc-400">{t("No sources found matching your criteria.")}</p>
            </div>
          ) : (
            filteredSources.map((source, idx) => (
              <div
                key={source.info_hash || idx}
                className="group flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3 rounded-2xl border border-zinc-800/80 bg-zinc-900/40 hover:bg-zinc-900/90 hover:border-zinc-700 transition-all"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-1.5 flex-wrap mb-1">
                    {getLangBadge(source)}
                    <span className="rounded-full bg-zinc-900 border border-zinc-800 px-1.5 py-0.5 text-[10px] font-mono text-zinc-400">
                      {source.quality}
                    </span>
                    {source.provider && <span className="text-[11px] text-zinc-400">{source.provider}</span>}
                    {source.release_group && source.release_group !== "Other" && (
                      <span className="text-[11px] font-mono text-zinc-400">
                        [{source.release_group}]
                      </span>
                    )}
                  </div>

                  <p
                    title={source.title}
                    className="text-xs font-mono text-zinc-200 group-hover:text-white transition-colors line-clamp-2 leading-relaxed"
                  >
                    {source.title}
                  </p>

                  <div className="flex flex-wrap items-center gap-3 mt-1.5 text-[11px] text-zinc-400 font-mono">
                    <div className="flex items-center gap-1 text-zinc-300">
                      <Users className="h-3 w-3 text-zinc-400" />
                      <span>{source.seeders} {" "}{t("seeds")}</span>
                    </div>

                    <div className="flex items-center gap-1 text-zinc-400">
                      <HardDrive className="h-3 w-3 text-zinc-500" />
                      <span>{source.size_display}</span>
                    </div>
                  </div>
                </div>

                {/* Play Button */}
                <div className="shrink-0 flex items-center justify-end">
                  <button
                    disabled={source.info_hash === currentSourceHash}
                    onClick={() => onSelectSource(source)}
                    className="flex items-center gap-1.5 rounded-full bg-zinc-100 hover:bg-white text-zinc-950 px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer"
                  >
                    <Play className="h-3.5 w-3.5 fill-current" />
                    <span>{t(source.info_hash === currentSourceHash ? "Source actuelle" : "Stream")}</span>
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
