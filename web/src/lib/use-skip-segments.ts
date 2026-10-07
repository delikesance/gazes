"use client";
import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { getSkipTimes } from "./api";
import { activeSkipSegment, mergeSkipSegments, needsAniSkip, shiftSegments, skipAction, type SkipSegment } from "./skip-segments";
import { episodeFileKey } from "./player-state";

interface SkipSegmentsInput {
  seasonId: number;
  episodeNumber?: number;
  infoHash?: string;
  fileIndex: number;
  /** Probed file duration in seconds, 0 while unknown. */
  durationSec: number;
  /** Chapter-based segments from the probe, in raw file time. */
  chapters?: SkipSegment[];
  /** HLS timeline origin to subtract from chapter times; 0 for the legacy remux. */
  origin: number;
  totalDuration: number;
  hasNextEpisode: boolean;
  playbackOffset: number;
  currentTimeRef: RefObject<number>;
}

/**
 * Opening/ending segments for the current file (chapters, completed by AniSkip when they miss one)
 * and the segment under the playhead. Positions are `playbackOffset + video.currentTime`.
 */
export function useSkipSegments({ seasonId, episodeNumber, infoHash, fileIndex, durationSec, chapters, origin, totalDuration, hasNextEpisode, playbackOffset, currentTimeRef }: SkipSegmentsInput) {
  const skipDuration = Math.round(durationSec || 0);
  const skipKey = skipDuration > 0 ? episodeFileKey(seasonId, episodeNumber, infoHash, fileIndex) : "";
  const chapterSkips = useMemo(() => shiftSegments(chapters ?? [], origin), [chapters, origin]);
  const wantsAniSkip = skipKey !== "" && needsAniSkip(chapterSkips);
  const [aniSkip, setAniSkip] = useState<{ key: string; segments: SkipSegment[] }>({ key: "", segments: [] });
  // Fire-and-forget: never on the startup path, errors resolve to [].
  useEffect(() => {
    if (!wantsAniSkip) return;
    const controller = new AbortController();
    const [season, episode] = skipKey.split(":").map(Number);
    void getSkipTimes(season, episode, skipDuration, controller.signal).then((segments) => {
      if (!controller.signal.aborted) setAniSkip({ key: skipKey, segments });
    });
    return () => controller.abort();
  }, [wantsAniSkip, skipKey, skipDuration]);
  const skipSegments = useMemo(
    () => mergeSkipSegments(chapterSkips, wantsAniSkip && aniSkip.key === skipKey ? aniSkip.segments : []),
    [chapterSkips, wantsAniSkip, aniSkip, skipKey],
  );
  const skipSegmentsRef = useRef<SkipSegment[]>([]);
  useEffect(() => { skipSegmentsRef.current = skipSegments; }, [skipSegments]);
  const [activeSkip, setActiveSkip] = useState<SkipSegment | null>(null);
  const activeSkipRef = useRef<SkipSegment | null>(null);
  // Called on every timeupdate; re-renders only when the active segment changes.
  const updateActiveSkip = useCallback((position: number) => {
    const next = activeSkipSegment(skipSegmentsRef.current, position);
    if (next === activeSkipRef.current) return;
    activeSkipRef.current = next;
    setActiveSkip(next);
  }, []);
  // A paused viewer inside an opening must see the button once late AniSkip results arrive.
  useEffect(() => {
    updateActiveSkip(playbackOffset + currentTimeRef.current);
  }, [skipSegments, playbackOffset, updateActiveSkip, currentTimeRef]);
  const shownSkip = activeSkip && skipSegments.includes(activeSkip) ? activeSkip : null;
  const shownSkipAction = shownSkip ? skipAction(shownSkip, totalDuration, hasNextEpisode) : "seek";
  return { shownSkip, shownSkipAction, updateActiveSkip };
}
