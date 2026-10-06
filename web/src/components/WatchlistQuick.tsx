"use client";
import { Check, Plus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { addToWatchlist, removeFromWatchlist, useInWatchlist } from "@/lib/watchlist";

/** Round add/remove-from-"ma liste" control that sits on top of a card without being inside its link. */
export function WatchlistQuick({ animeId, title, className = "" }: { animeId: number; title: string; className?: string }) {
  const { t } = useI18n();
  const saved = useInWatchlist(animeId);
  return <button type="button" className={`watchlist-quick ${className}`} aria-pressed={saved}
    aria-label={t(saved ? "Retirer {title} de ma liste" : "Ajouter {title} à ma liste", { title })}
    onClick={() => (saved ? removeFromWatchlist(animeId) : addToWatchlist(animeId))}>
    {saved ? <Check size={16} aria-hidden="true" /> : <Plus size={16} aria-hidden="true" />}
  </button>;
}
