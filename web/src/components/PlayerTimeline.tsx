"use client";
import { useState, type RefObject } from "react";
import { useI18n } from "@/lib/i18n";
import { formatTime, scrubberTarget } from "@/lib/player-state";

interface PlayerTimelineProps {
  // Elements the parent repaints on every timeupdate, outside React, for a smooth timeline.
  timeRef: RefObject<HTMLSpanElement | null>;
  barRef: RefObject<HTMLDivElement | null>;
  bufferedRef: RefObject<HTMLDivElement | null>;
  playedRef: RefObject<HTMLDivElement | null>;
  knobRef: RefObject<HTMLDivElement | null>;
  playbackOffset: number;
  totalDuration: number;
  /** Shown while the duration is unknown. */
  formattedDuration?: string;
  /** Current position on the timeline, read on key presses. */
  position: () => number;
  onSeek: (seconds: number) => void;
}

/** The scrubber: elapsed time, seek bar with hover preview, total duration. */
export function PlayerTimeline({ timeRef, barRef, bufferedRef, playedRef, knobRef, playbackOffset, totalDuration, formattedDuration, position, onSeek }: PlayerTimelineProps) {
  const { t } = useI18n();
  const [hoverTime, setHoverTime] = useState<number | null>(null);
  const [hoverPosition, setHoverPosition] = useState<number>(0);

  const ratioAt = (clientX: number) => {
    const rect = barRef.current!.getBoundingClientRect();
    return Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
  };

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!barRef.current || totalDuration <= 0) return;
    onSeek(ratioAt(e.clientX) * totalDuration);
  };

  // The scrubber is a slider for keyboards and screen readers: arrows seek 5 s, Page keys 30 s.
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (totalDuration <= 0 || e.ctrlKey || e.metaKey || e.altKey) return;
    const target = scrubberTarget(e.key, position(), totalDuration);
    if (target === null) return;
    // The window-level shortcuts would seek a second time.
    e.preventDefault();
    e.stopPropagation();
    onSeek(target);
  };

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!barRef.current || totalDuration <= 0) return;
    const ratio = ratioAt(e.clientX);
    setHoverPosition(ratio * 100);
    setHoverTime(ratio * totalDuration);
  };

  return (
    <div className="flex items-center gap-3.5 px-1.5">
      <span ref={timeRef} className="min-w-11 font-mono text-xs text-zinc-50">{formatTime(playbackOffset)}</span>
      <div
        ref={barRef}
        onClick={handleClick}
        role="slider"
        tabIndex={0}
        aria-label={t("Position de lecture")}
        aria-valuemin={0}
        aria-valuenow={0} /* moved by the parent's updateProgressDisplay; React never rewrites an unchanged prop */
        aria-valuemax={Math.floor(totalDuration)}
        onKeyDown={handleKeyDown}
        onMouseMove={handleMouseMove}
        onMouseLeave={() => setHoverTime(null)}
        className="group/bar relative flex h-5 flex-1 cursor-pointer items-center"
      >
        <div className="relative h-1.5 w-full rounded-full bg-white/20 transition-all group-hover/bar:h-2">
          <div ref={bufferedRef} className="absolute top-0 h-full rounded-full bg-white/55" style={{ left: "0%", width: "0%" }} />
          <div ref={playedRef} className="absolute left-0 top-0 h-full rounded-full bg-[var(--accent)]" style={{ width: "0%" }} />
          <div
            ref={knobRef}
            className="absolute top-1/2 -ml-2 -mt-2 h-4 w-4 rounded-full bg-white shadow-[0_0_0_5px_color-mix(in_srgb,var(--accent)_30%,transparent)]"
            style={{ left: "0%" }}
          />
        </div>
        {hoverTime !== null && (
          <div
            className="player-frost pointer-events-none absolute bottom-6 -translate-x-1/2 rounded-full px-2.5 py-0.5 font-mono text-[11px] text-white"
            style={{ left: `${hoverPosition}%` }}
          >
            {formatTime(hoverTime)}
          </div>
        )}
      </div>
      <span className="min-w-11 text-right font-mono text-xs text-zinc-400">
        {totalDuration > 0 ? formatTime(totalDuration) : formattedDuration || "--:--"}
      </span>
    </div>
  );
}
