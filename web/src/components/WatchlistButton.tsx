"use client";
import { Check, Plus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { addToWatchlist, removeFromWatchlist, useInWatchlist } from "@/lib/watchlist";

export function WatchlistButton({ animeId }: { animeId: number }) {
  const { t } = useI18n();
  const saved = useInWatchlist(animeId);
  return <button type="button" className="clay clay-secondary watchlist-button" aria-pressed={saved}
    onClick={() => (saved ? removeFromWatchlist(animeId) : addToWatchlist(animeId))}>
    {saved ? <Check size={16} aria-hidden="true" /> : <Plus size={16} aria-hidden="true" />}
    {saved ? t("Dans ma liste") : t("Ma liste")}
  </button>;
}
