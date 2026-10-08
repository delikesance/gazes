"use client";
import { useEffect, useRef, type RefObject } from "react";
import type { LoadTorrentResponse } from "@/types/api";
import type { PlaybackDiagnostic } from "./diagnostics";
import { HlsPlaybackController } from "./hls-playback";

export type HlsCallbacks = ConstructorParameters<typeof HlsPlaybackController>[1];

interface HlsPlaybackInput {
  hlsMode: boolean;
  loading: boolean;
  needsFileSelection: boolean;
  loadData: LoadTorrentResponse | null;
  fileIndex: number;
  audioTrack: number;
  videoRef: RefObject<HTMLVideoElement | null>;
  /** Holds the live controller, for play/pause/seek and position reads. */
  controllerRef: RefObject<HlsPlaybackController | null>;
  /** False when the viewer last paused: the new controller opens paused. */
  resumePlaybackRef: RefObject<boolean>;
  /** Controller callbacks for the video element it drives; read when a controller is created. */
  callbacks: (video: HTMLVideoElement) => HlsCallbacks;
  diagnostic?: PlaybackDiagnostic;
  initialTime: number;
  itemHash: string | null;
}

/**
 * One HLS controller per file and audio track, opened where the previous one left off.
 * Returns the resume position, which an audio switch sets before the controller is replaced.
 */
export function useHlsPlayback({ hlsMode, loading, needsFileSelection, loadData, fileIndex, audioTrack, videoRef, controllerRef, resumePlaybackRef, callbacks, diagnostic, initialTime, itemHash }: HlsPlaybackInput) {
  const hlsResumePosition = useRef(initialTime);
  useEffect(() => {
    if (!hlsMode || loading || needsFileSelection || !loadData || fileIndex < 0 || !videoRef.current) return;
    const video = videoRef.current;
    const controller = new HlsPlaybackController(video, callbacks(video), diagnostic, !resumePlaybackRef.current);
    controllerRef.current = controller;
    const position = hlsResumePosition.current;
    void controller.open(loadData.info_hash, fileIndex, audioTrack, position);
    return () => { hlsResumePosition.current = video.currentTime || position; controller.dispose(); if (controllerRef.current === controller) controllerRef.current = null; };
  }, [hlsMode, loadData, fileIndex, audioTrack, loading, needsFileSelection]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { hlsResumePosition.current = initialTime; }, [itemHash, initialTime]);
  return hlsResumePosition;
}
