"use client";
import { useEffect, useState } from "react";

/** Seconds left before the next episode starts on its own; null while idle or cancelled. */
export function useNextEpisodeCountdown(isPlaying: boolean, onNextEpisode?: () => void) {
  const [nextCountdown, setNextCountdown] = useState<number | null>(null);
  // Playing again (replay, a party member resuming) cancels the countdown.
  if (isPlaying && nextCountdown !== null) setNextCountdown(null);
  useEffect(() => {
    if (nextCountdown === null) return;
    const timer = window.setTimeout(() => {
      if (nextCountdown <= 1) { setNextCountdown(null); onNextEpisode?.(); } else setNextCountdown(nextCountdown - 1);
    }, 1000);
    return () => window.clearTimeout(timer);
  }, [nextCountdown, onNextEpisode]);
  return [nextCountdown, setNextCountdown] as const;
}
