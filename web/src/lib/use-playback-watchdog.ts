"use client";
import { useEffect, type RefObject } from "react";
import type { EpisodeSource, TorrentItem } from "@/types/api";
import { getTorrentStats } from "./api";
import type { PlaybackDiagnostic } from "./diagnostics";
import { PLAYBACK_TIMEOUTS } from "./playback-sources";
import { metadataTimedOut, streamWatchdog } from "./player-state";

interface WatchdogInput {
  item: TorrentItem | null;
  diagnostic?: PlaybackDiagnostic;
  /** Off when nobody listens for failures (no automatic failover). */
  enabled: boolean;
  hlsMode: boolean;
  loading: boolean;
  needsFileSelection: boolean;
  reportFailure: (reason: string, code: string) => void;
  videoRef: RefObject<HTMLVideoElement | null>;
  resumePlaybackRef: RefObject<boolean>;
  hasStartedRef: RefObject<boolean>;
  lastProgressRef: RefObject<{ time: number; at: number }>;
  /** A new remux stream (seek or audio change) restarts the stream phase. */
  timeOffset: number;
  audioTrack: number;
}

/** Fails a legacy source whose swarm never delivers metadata, never starts, or stalls for good. */
export function usePlaybackWatchdog({ item, diagnostic, enabled, hlsMode, loading, needsFileSelection, reportFailure, videoRef, resumePlaybackRef, hasStartedRef, lastProgressRef, timeOffset, audioTrack }: WatchdogInput) {
  // Metadata phase: wait as long as the swarm is alive (a connected peer or incoming bytes).
  useEffect(() => {
    if (!enabled || !item || !loading || (item as EpisodeSource).library) return;
    const startedAt = Date.now();
    let lastActivity = startedAt;
    let lastBytes = 0;
    const check = setInterval(() => {
      getTorrentStats(item.info_hash, diagnostic).then((s) => {
        // A connected peer alone does not prove metadata is arriving.
        if (s.completed_bytes > lastBytes) lastActivity = Date.now();
        lastBytes = s.completed_bytes;
      }).catch(() => {});
      const now = Date.now();
      if (metadataTimedOut(now, startedAt, lastActivity, PLAYBACK_TIMEOUTS)) {
        reportFailure("Cette source ne fournit pas ses métadonnées à temps.", "metadata_timeout");
      }
    }, 2000);
    return () => clearInterval(check);
  }, [loading, item, enabled, reportFailure, diagnostic]);

  // Actual time advancement proves playback. Buffering can occur without an
  // error event, so a dead swarm must not hold this source indefinitely.
  useEffect(() => {
    if (hlsMode || !enabled || loading || needsFileSelection) return;
    const startedAt = Date.now();
    let lastActivity = startedAt;
    let lastBytes = -1;
    let lastBuffered = 0;
    let tick = 0;
    const poll = setInterval(() => {
      const video = videoRef.current;
      if (!video || !resumePlaybackRef.current) return;
      const now = Date.now();
      // Activity = new bytes from the swarm or new buffered video: slow is fine, stuck is not.
      const buffered = video.buffered.length ? video.buffered.end(video.buffered.length - 1) : 0;
      if (buffered > lastBuffered) lastActivity = now;
      lastBuffered = buffered;
      if (item && !(item as EpisodeSource).library && ++tick % 2 === 0) {
        getTorrentStats(item.info_hash, diagnostic).then((s) => {
          if (lastBytes >= 0 && s.completed_bytes > lastBytes) lastActivity = Date.now();
          lastBytes = s.completed_bytes;
        }).catch(() => {});
      }
      const failure = streamWatchdog({ now, startedAt, lastActivity, lastProgressAt: lastProgressRef.current.at, hasStarted: hasStartedRef.current }, PLAYBACK_TIMEOUTS);
      if (failure) reportFailure(failure.reason, failure.code);
    }, 1000);
    return () => clearInterval(poll);
  }, [hlsMode, loading, needsFileSelection, enabled, reportFailure, timeOffset, audioTrack, item, diagnostic, videoRef, resumePlaybackRef, hasStartedRef, lastProgressRef]);
}
