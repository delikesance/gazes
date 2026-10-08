"use client";
import { Cast, Link2, List, Maximize, Minimize, Pause, PictureInPicture2, Play, RotateCcw, RotateCw, Settings, SkipBack, SkipForward, Users, Volume1, Volume2, VolumeX } from "lucide-react";
import { useI18n } from "@/lib/i18n";

interface PlayerControlsProps {
  isPlaying: boolean;
  onTogglePlay: () => void;
  onPrevEpisode?: () => void;
  onNextEpisode?: () => void;
  /** Relative seek, in seconds from the current position. */
  onSeekBy: (delta: number) => void;
  isMuted: boolean;
  volume: number;
  onToggleMute: () => void;
  onVolumeChange: (volume: number) => void;
  /** Absent when there is no episode list to show. */
  onToggleEpisodes?: () => void;
  showEpisodes: boolean;
  optionsOpen: boolean;
  onToggleOptions: () => void;
  partyActive: boolean;
  partyCopied: boolean;
  onCopyPartyLink: () => void;
  linkCopied: boolean;
  onCopyTimeLink: () => void;
  /** Absent when the browser has no AirPlay or picture-in-picture. */
  onAirPlay?: () => void;
  onPip?: () => void;
  isFullscreen: boolean;
  onToggleFullscreen: () => void;
}

/** The dock's button row: transport and volume on the left, panels and display modes on the right. */
export function PlayerControls({ isPlaying, onTogglePlay, onPrevEpisode, onNextEpisode, onSeekBy, isMuted, volume, onToggleMute, onVolumeChange, onToggleEpisodes, showEpisodes, optionsOpen, onToggleOptions, partyActive, partyCopied, onCopyPartyLink, linkCopied, onCopyTimeLink, onAirPlay, onPip, isFullscreen, onToggleFullscreen }: PlayerControlsProps) {
  const { t } = useI18n();
  return (
    <div className="player-controls-row flex items-center justify-between gap-3 text-xs">
      <div className="flex items-center gap-2">
        <button onClick={onTogglePlay} className="player-pill player-pill--icon player-pill--solid" aria-label={isPlaying ? t("Pause (Space)") : t("Play (Space)")} title={isPlaying ? t("Pause (Space)") : t("Play (Space)")}>
          {isPlaying ? <Pause className="h-5 w-5 fill-current" /> : <Play className="h-5 w-5 fill-current" />}
        </button>
        {onPrevEpisode && (
          <button onClick={onPrevEpisode} className="player-pill player-pill--icon" aria-label={t("Previous Episode")} title={t("Previous Episode")}>
            <SkipBack className="h-[18px] w-[18px]" />
          </button>
        )}
        {onNextEpisode && (
          <button onClick={onNextEpisode} className="player-pill player-pill--icon" aria-label={t("Next Episode")} title={t("Next Episode")}>
            <SkipForward className="h-[18px] w-[18px]" />
          </button>
        )}
        <button onClick={() => onSeekBy(-10)} className="player-pill player-pill--icon" aria-label={t("Rewind 10s (←)")} title={t("Rewind 10s (←)")}>
          <RotateCcw className="h-[18px] w-[18px]" />
        </button>
        <button onClick={() => onSeekBy(10)} className="player-pill player-pill--icon" aria-label={t("Forward 10s (→)")} title={t("Forward 10s (→)")}>
          <RotateCw className="h-[18px] w-[18px]" />
        </button>
        {/* Volume */}
        <div className="hidden items-center gap-3 rounded-full bg-white/[.06] pr-5 sm:flex">
          <button onClick={onToggleMute} className="player-pill player-pill--icon" aria-label={t(isMuted ? "Unmute (M)" : "Mute (M)")} title={t(isMuted ? "Unmute (M)" : "Mute (M)")}>
            {isMuted || volume === 0 ? <VolumeX className="h-[18px] w-[18px]" /> : volume < 0.5 ? <Volume1 className="h-[18px] w-[18px]" /> : <Volume2 className="h-[18px] w-[18px]" />}
          </button>
          <input
            aria-label={t("Volume")}
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={isMuted ? 0 : volume}
            onChange={(e) => onVolumeChange(parseFloat(e.target.value))}
            style={{ background: `linear-gradient(to right, var(--accent) ${(isMuted ? 0 : volume) * 100}%, rgb(255 255 255 / 0.2) ${(isMuted ? 0 : volume) * 100}%)` }}
            className="h-1.5 w-20 cursor-pointer appearance-none rounded-full accent-white"
          />
        </div>
      </div>

      {/* Right: Episodes, Settings, Fullscreen */}
      <div className="flex items-center gap-2">
        {onToggleEpisodes && (
          <button
            onClick={onToggleEpisodes}
            data-active={showEpisodes}
            aria-expanded={showEpisodes}
            className="player-pill player-pill--collapse"
            title={t("Épisodes")}
          >
            <List className="h-4 w-4" /><span className="player-label">{t("Épisodes")}</span>
          </button>
        )}
        <button
          onClick={onToggleOptions}
          data-active={optionsOpen}
          aria-haspopup="dialog"
          className="player-pill player-pill--collapse"
          title={t("Select Audio Track")}
        >
          <Settings className="h-4 w-4" /><span className="player-label">{t("Réglages")}</span>
        </button>
        <button onClick={onCopyPartyLink} className="player-pill player-pill--icon" data-active={partyActive} aria-live="polite" aria-label={t(partyCopied ? "Lien copié" : "Regarder ensemble : copier l’invitation")} title={t(partyCopied ? "Lien copié" : "Regarder ensemble : copier l’invitation")}>
          <Users className="h-[18px] w-[18px]" />
        </button>
        <button onClick={onCopyTimeLink} className="player-pill player-pill--icon" aria-live="polite" aria-label={t(linkCopied ? "Lien copié" : "Copier le lien à cet instant")} title={t(linkCopied ? "Lien copié" : "Copier le lien à cet instant")}>
          <Link2 className="h-[18px] w-[18px]" />
        </button>
        {onAirPlay && (
          <button onClick={onAirPlay} className="player-pill player-pill--icon" aria-label={t("AirPlay")} title={t("AirPlay")}>
            <Cast className="h-[18px] w-[18px]" />
          </button>
        )}
        {onPip && (
          <button onClick={onPip} className="player-pill player-pill--icon" aria-label={t("Picture-in-Picture (P)")} title={t("Picture-in-Picture (P)")}>
            <PictureInPicture2 className="h-[18px] w-[18px]" />
          </button>
        )}
        <button onClick={onToggleFullscreen} className="player-pill player-pill--icon" aria-label={t("Toggle Fullscreen (F)")} title={t("Toggle Fullscreen (F)")}>
          {isFullscreen ? <Minimize className="h-[18px] w-[18px]" /> : <Maximize className="h-[18px] w-[18px]" />}
        </button>
      </div>
    </div>
  );
}
