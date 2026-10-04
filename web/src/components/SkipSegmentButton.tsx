"use client";
import { SkipForward } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { SkipSegment } from "@/lib/skip-segments";

/** Floating skip action shown while an opening or an ending plays. */
export function SkipSegmentButton({ segment, action, onSkip }: { segment: SkipSegment; action: "seek" | "next-episode"; onSkip: () => void }) {
  const { t } = useI18n();
  const label = action === "next-episode" ? t("Épisode suivant") : segment.kind === "opening" ? t("Passer l'opening") : t("Passer l'ending");
  return (
    <button type="button" onClick={onSkip} className="player-skip player-pill player-pill--solid" aria-keyshortcuts="S" title={`${label} (S)`}>
      <span>{label}</span>
      <SkipForward className="h-4 w-4" aria-hidden />
    </button>
  );
}
