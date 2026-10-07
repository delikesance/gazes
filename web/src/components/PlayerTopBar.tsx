"use client";
import { ArrowLeft, Check, Image as ImageIcon, Layers, X } from "lucide-react";
import { formatBytes } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { SwarmStats, TorrentItem } from "@/types/api";

interface PlayerTopBarProps {
  item: TorrentItem;
  animeTitle?: string;
  episodeNumber?: number;
  pageMode: boolean;
  showControls: boolean;
  previewSaved: boolean;
  stats: SwarmStats | null;
  onActivity: () => void;
  onClose: () => void;
  onChangeSource?: () => void;
}

/** Close/back button, episode title and swarm summary above the video. */
export function PlayerTopBar({ item, animeTitle, episodeNumber, pageMode, showControls, previewSaved, stats, onActivity, onClose, onChangeSource }: PlayerTopBarProps) {
  const { t } = useI18n();
  return (
    <div onMouseMove={onActivity} className={`player-heading player-topbar${!showControls?" watch-heading-hidden":""}`}>
      <div className="flex min-w-0 items-center gap-2.5">
        <button
          aria-label={t(pageMode?"Voir les saisons":"Fermer le lecteur")}
          onClick={onClose}
          className="player-frost player-pill player-pill--icon shrink-0"
        >
          {pageMode ? <ArrowLeft className="h-5 w-5" aria-hidden="true" /> : <X className="h-5 w-5" aria-hidden="true" />}
        </button>
        <div className="player-frost player-pill player-episode-pill min-w-0">
          {episodeNumber && <span className="player-chip player-chip--solid">EP {episodeNumber}</span>}
          {previewSaved && <span role="status" className="player-chip player-chip--solid player-chip--preview"><ImageIcon size={13} aria-hidden="true" /><Check size={13} aria-hidden="true" />{t("Aperçu enregistré")}</span>}
          <h2 className="min-w-0 truncate text-[13px] font-medium" title={animeTitle || item.title}>
            {animeTitle || item.anime_details?.display_title || item.title}
          </h2>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2.5">
        {stats && (
          <div className="player-frost player-pill hidden !gap-2 sm:inline-flex" role="status">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" aria-hidden="true" />
            <span className="font-mono text-xs">{stats.active_seeders} {t("Seeders")} · {formatBytes(stats.download_rate_bps)}/s</span>
          </div>
        )}
        {onChangeSource && (
          <button onClick={onChangeSource} aria-label={t("Changer de source pour cet épisode")} title={t("Changer de source pour cet épisode")} className="player-frost player-pill player-pill--collapse">
            <Layers className="h-4 w-4" /><span className="player-label">{t("Sources")}</span>
          </button>
        )}
      </div>
    </div>
  );
}
