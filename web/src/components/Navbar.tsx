"use client";
import { useI18n } from "@/lib/i18n";

import React, { useState } from "react";
import { Search, X } from "lucide-react";

interface NavbarProps {
  onSearch: (query: string) => void;
  initialQuery?: string;
}

export const Navbar: React.FC<NavbarProps> = ({ onSearch, initialQuery = "" }) => {
  const { t } = useI18n();
  const [query, setQuery] = useState(initialQuery);
  const [prevInitialQuery, setPrevInitialQuery] = useState(initialQuery);

  if (prevInitialQuery !== initialQuery) {
    setPrevInitialQuery(initialQuery);
    setQuery(initialQuery);
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSearch(query.trim());
  };

  const handleClear = () => {
    setQuery("");
    onSearch("");
  };

  return (
    <header className="sticky top-0 z-30 w-full border-b border-zinc-800 bg-zinc-950/90 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8 gap-4">
        {/* Brand / Logo */}
        <div
          className="flex items-center gap-3 cursor-pointer group"
          onClick={() => {
            setQuery("");
            onSearch("");
          }}
        >
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-100 text-zinc-950 font-bold text-sm tracking-tighter">
            G
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="text-base font-semibold tracking-tight text-zinc-100 group-hover:text-white transition-colors">
                GAZES
              </span>
              <span className="text-[10px] font-mono text-zinc-400 bg-zinc-900 border border-zinc-800 px-1.5 py-0.2 rounded">
                STREAM
              </span>
            </div>
          </div>
        </div>

        {/* Search Bar */}
        <div className="flex-1 max-w-lg mx-2 sm:mx-6">
          <form onSubmit={handleSubmit} className="relative flex items-center">
            <Search className="absolute left-3.5 h-4 w-4 text-zinc-400 pointer-events-none" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("Search anime (e.g. Frieren, Dandadan, One Piece)...")}
              className="w-full rounded-lg border border-zinc-800 bg-zinc-900/80 py-2 pl-10 pr-20 text-xs sm:text-sm text-zinc-100 placeholder-zinc-400 transition-all focus:border-zinc-500 focus:bg-zinc-900 focus:outline-none"
            />
            {query && (
              <button
                type="button"
                onClick={handleClear}
                className="absolute right-12 p-1 text-zinc-400 hover:text-zinc-200 transition-colors"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
            <button
              type="submit"
              className="absolute right-1.5 px-2.5 py-1 text-xs font-medium text-zinc-300 hover:text-white bg-zinc-800 hover:bg-zinc-700 rounded transition-all cursor-pointer"
            >
              {t("Search")}</button>
          </form>
        </div>

        {/* Engine Status */}
        <div className="hidden sm:flex items-center gap-2 text-xs text-zinc-400 font-mono">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
          <span>{t("Swarm Engine")}</span>
        </div>
      </div>
    </header>
  );
};
