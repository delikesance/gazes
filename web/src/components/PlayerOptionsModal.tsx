"use client";
import { useEffect } from "react";
import { Check, X } from "lucide-react";
import { useI18n } from "@/lib/i18n";

export type PlayerOptionsTab = "audio" | "subtitles" | "ambilight";

export type AmbilightLevel = "soft" | "medium" | "vivid";
export interface AmbilightSettings { on: boolean; level: AmbilightLevel; dim: boolean }

export interface PlayerTrackOption { index: number; label: string; title?: string }

interface PlayerOptionsModalProps {
  tab: PlayerOptionsTab;
  onTabChange: (tab: PlayerOptionsTab) => void;
  onClose: () => void;
  audioOptions: PlayerTrackOption[];
  selectedAudio: number;
  audioHint?: string | null;
  onSelectAudio: (index: number) => void;
  subtitleOptions: PlayerTrackOption[];
  selectedSubtitle: number | null;
  onSelectSubtitle: (index: number | null) => void;
  ambilight: AmbilightSettings;
  onAmbilightChange: (patch: Partial<AmbilightSettings>) => void;
}

/** One frosted dialog for the tracks a viewer picks: audio and subtitles. */
export function PlayerOptionsModal({ tab, onTabChange, onClose, audioOptions, selectedAudio, audioHint, onSelectAudio, subtitleOptions, selectedSubtitle, onSelectSubtitle, ambilight, onAmbilightChange }: PlayerOptionsModalProps) {
  const { t } = useI18n();
  const tabs: [PlayerOptionsTab, string][] = [["audio", "Audio"], ["subtitles", "Sous-titres"], ["ambilight", "Ambilight"]];

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") { event.stopPropagation(); onClose(); } };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  return (
    <div className="absolute inset-0 z-40 flex items-center justify-center bg-black/45 p-4 sm:p-8" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label={t("Réglages")} onClick={(event) => event.stopPropagation()} className="player-panel player-modal flex max-h-full w-full max-w-[520px] flex-col gap-4 !p-4 sm:!p-5 animate-in fade-in duration-150">
        <div className="flex items-center justify-between gap-3">
          <div role="tablist" className="flex gap-1 rounded-full bg-white/[.08] p-1">
            {tabs.map(([id, label]) => (
              <button key={id} role="tab" aria-selected={tab === id} data-active={tab === id} onClick={() => onTabChange(id)} className="player-row !min-h-9 !w-auto shrink-0 !justify-center whitespace-nowrap !px-5">{t(label)}</button>
            ))}
          </div>
          <button type="button" className="player-pill player-pill--sm player-pill--icon" aria-label={t("Fermer")} onClick={onClose}><X className="h-4 w-4" /></button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {tab === "audio" && (
            <div className="flex flex-col gap-1">
              <p className="player-panel-title">{t("Audio Tracks")}</p>
              {audioHint && <p className="px-4 py-2 text-xs text-zinc-400">{audioHint}</p>}
              {audioOptions.length > 0 ? audioOptions.map((option) => (
                <button key={option.index} onClick={() => onSelectAudio(option.index)} data-active={selectedAudio === option.index} className="player-row">
                  <span className="truncate" title={option.title}>{option.label}</span>
                  {selectedAudio === option.index && <Check className="h-4 w-4 shrink-0" />}
                </button>
              )) : <div className="px-4 py-2 text-xs text-zinc-400">{t("Default Audio")}</div>}
            </div>
          )}

          {tab === "subtitles" && (
            <div className="flex flex-col gap-1">
              <p className="player-panel-title">{t("Subtitles")}</p>
              <button onClick={() => onSelectSubtitle(null)} data-active={selectedSubtitle === null} className="player-row">
                <span>{t("Off")}</span>
                {selectedSubtitle === null && <Check className="h-4 w-4 shrink-0" />}
              </button>
              {subtitleOptions.map((option) => (
                <button key={option.index} onClick={() => onSelectSubtitle(option.index)} data-active={selectedSubtitle === option.index} className="player-row">
                  <span className="truncate" title={option.title}>{option.label}</span>
                  {selectedSubtitle === option.index && <Check className="h-4 w-4 shrink-0" />}
                </button>
              ))}
            </div>
          )}

          {tab === "ambilight" && (
            <div className="flex flex-col gap-1">
              <p className="player-panel-title">{t("Ambilight")}</p>
              <div className="flex items-center justify-between gap-4 px-4 py-1.5">
                <div><div className="text-sm font-medium">{t("Ambilight")}</div><div className="mt-0.5 text-xs text-zinc-400">{t("Lueur autour de la vidéo")}</div></div>
                <button role="switch" aria-checked={ambilight.on} aria-label={t("Ambilight")} onClick={() => onAmbilightChange({ on: !ambilight.on })} className="player-switch" data-on={ambilight.on} />
              </div>
              <div className="px-3 py-1.5">
                <div className="mb-2 px-1 text-xs text-zinc-400">{t("Intensité")}</div>
                <div className="flex gap-1 rounded-full bg-white/[.08] p-1" role="group" aria-label={t("Intensité")}>
                  {([["soft", "Douce"], ["medium", "Moyenne"], ["vivid", "Vive"]] as const).map(([level, label]) => (
                    <button key={level} disabled={!ambilight.on} aria-pressed={ambilight.level === level} data-active={ambilight.level === level} onClick={() => onAmbilightChange({ level })} className="player-row !min-h-9 flex-1 !justify-center !px-2 disabled:opacity-40">{t(label)}</button>
                  ))}
                </div>
              </div>
              <div className="flex items-center justify-between gap-4 px-4 py-1.5">
                <div><div className="text-sm font-medium">{t("Atténuer en pause")}</div><div className="mt-0.5 text-xs text-zinc-400">{t("Fondu après 3 s")}</div></div>
                <button role="switch" aria-checked={ambilight.dim} aria-label={t("Atténuer en pause")} disabled={!ambilight.on} onClick={() => onAmbilightChange({ dim: !ambilight.dim })} className="player-switch" data-on={ambilight.dim} />
              </div>
              <p className="mt-1 border-t border-white/10 px-4 pb-1.5 pt-3 text-xs text-zinc-400">{t("Désactivé automatiquement en plein écran.")}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
