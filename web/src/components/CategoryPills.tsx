"use client";
import { useI18n } from "@/lib/i18n";

import React from "react";
import { Sparkles, Globe, ShieldCheck, Flame } from "lucide-react";

interface CategoryPillsProps {
  currentCategory: string;
  onSelectCategory: (cat: string) => void;
  currentSort: string;
  onSelectSort: (sort: string) => void;
}

const categories = [
  { id: "1_2", label: "English-Translated", icon: Globe },
  { id: "1_0", label: "All Anime", icon: Sparkles },
  { id: "1_3", label: "Non-English Subbed", icon: ShieldCheck },
  { id: "1_4", label: "Raw Anime", icon: Flame },
];

export const CategoryPills: React.FC<CategoryPillsProps> = ({
  currentCategory,
  onSelectCategory,
  currentSort,
  onSelectSort,
}) => {
  const { t } = useI18n();
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 pb-3 pt-2">
      {/* Category Pills */}
      <div className="flex flex-wrap items-center gap-1.5">
        {categories.map((cat) => {
          const Icon = cat.icon;
          const isActive = currentCategory === cat.id;
          return (
            <button
              key={cat.id}
              onClick={() => onSelectCategory(cat.id)}
              className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer ${
                isActive
                  ? "bg-zinc-100 text-zinc-950"
                  : "bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200 border border-zinc-800"
              }`}
            >
              <Icon className="h-3 w-3" />
              <span>{t(cat.label)}</span>
            </button>
          );
        })}
      </div>

      {/* Sort Selector */}
      <div className="flex items-center gap-2 text-xs text-zinc-400 font-mono">
        <span>{t("Sort:")}</span>
        <select
          value={currentSort}
          onChange={(e) => onSelectSort(e.target.value)}
          className="rounded-full border border-zinc-800 bg-zinc-900 px-2 py-1 text-xs text-zinc-200 focus:border-zinc-500 focus:outline-none"
        >
          <option value="seeders">{t("Seeders")}</option>
          <option value="id">{t("Latest")}</option>
          <option value="size">{t("Size")}</option>
        </select>
      </div>
    </div>
  );
};
