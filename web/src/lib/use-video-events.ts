"use client";
import type { RefObject, SyntheticEvent } from "react";
import type { LoadTorrentResponse, TorrentItem, VideoMetadata } from "@/types/api";
import { diagnosticEvent, type PlaybackDiagnostic } from "./diagnostics";
import { NEXT_EPISODE_DELAY, endedEarly, isLibrarySource, mediaErrorMessage, progressTracks } from "./player-state";

type VideoEvent = SyntheticEvent<HTMLVideoElement>;

interface VideoEventsInput {
  item: TorrentItem | null;
  diagnostic?: PlaybackDiagnostic;
  hlsMode: boolean;
  videoRef: RefObject<HTMLVideoElement | null>;
  resumePlaybackRef: RefObject<boolean>;
  hasStartedRef: RefObject<boolean>;
  lastProgressRef: RefObject<{ time: number; at: number }>;
  loadStartedAtRef: RefObject<number | null>;
  currentTimeRef: RefObject<number>;
  fileResolvedRef: RefObject<boolean>;
  onFileResolvedRef: RefObject<((infoHash: string, fileIndex: number) => void) | undefined>;
  loadData: LoadTorrentResponse | null;
  selectedFileIdx: number;
  videoMeta: VideoMetadata | null;
  audioTrack: number;
  subtitleTrack: number | null;
  playbackOffset: number;
  totalDuration: number;
  volume: number;
  isMuted: boolean;
  onProgress?: (position: number, duration: number, tracks?: { audioLang?: string; subLang?: string }) => void;
  onPlaybackFailure?: (failure: { reason: string; position: number }) => void;
  onNextEpisode?: () => void;
  reportFailure: (reason: string, code?: string) => void;
  updateProgressDisplay: (timeSec: number) => void;
  updateBuffered: () => void;
  updateActiveSkip: (position: number) => void;
  setIsPlaying: (playing: boolean) => void;
  setIsBuffering: (buffering: boolean) => void;
  setStarted: (started: boolean) => void;
  setNeedsPlaybackGesture: (needed: boolean) => void;
  setPlaybackError: (message: string | null) => void;
  setNextCountdown: (seconds: number | null) => void;
}

/**
 * Media event handlers for the player's <video>: progress and diagnostics, autoplay
 * (legacy engine), media errors and the end of the episode. Events from a replaced
 * element (a new legacy stream remounts it) are ignored.
 */
export function useVideoEvents(c: VideoEventsInput) {
  const { diagnostic, hlsMode, videoRef, resumePlaybackRef, hasStartedRef, lastProgressRef, loadStartedAtRef, currentTimeRef, playbackOffset, totalDuration, videoMeta } = c;
  return {
    onTimeUpdate: (event: VideoEvent) => {
      if (hlsMode && event.currentTarget.dataset.playbackPhase === "preparing") return;
      if(!hasStartedRef.current&&event.currentTarget===videoRef.current&&event.currentTarget.currentTime>lastProgressRef.current.time&&event.currentTarget.videoWidth>0){const startedAt=loadStartedAtRef.current;loadStartedAtRef.current=null;diagnosticEvent(diagnostic,"playback.started",{position:playbackOffset+event.currentTarget.currentTime,width:event.currentTarget.videoWidth,height:event.currentTarget.videoHeight,...(startedAt!==null?{startup_ms:Math.round(performance.now()-startedAt)}:{})});}
      if (event.currentTarget === videoRef.current) {
        const time = videoRef.current.currentTime;
        if (time > lastProgressRef.current.time && videoRef.current.videoWidth > 0) {
          hasStartedRef.current = true;
          lastProgressRef.current = { time, at: Date.now() };
        }
        currentTimeRef.current = time;
        c.onProgress?.(playbackOffset + time, totalDuration, progressTracks(videoMeta, c.audioTrack, c.subtitleTrack));
        c.updateProgressDisplay(videoRef.current.currentTime);
        c.updateBuffered();
        c.updateActiveSkip(playbackOffset + time);
      }
    },
    onPlay: (event: VideoEvent) => { if (event.currentTarget === videoRef.current) {resumePlaybackRef.current=true;c.setNeedsPlaybackGesture(false);c.setIsPlaying(true);} },
    onPause: (event: VideoEvent) => { if (event.currentTarget === videoRef.current) c.setIsPlaying(false); },
    onWaiting: (event: VideoEvent) => { if (event.currentTarget === videoRef.current) {c.setIsBuffering(true);diagnosticEvent(diagnostic,"playback.buffering",{position:playbackOffset+event.currentTarget.currentTime,ready_state:event.currentTarget.readyState});} },
    onProgress: () => c.updateBuffered(),
    onPlaying: (event: VideoEvent) => { if (event.currentTarget === videoRef.current) {
      c.setIsBuffering(false); c.setStarted(true);
      if (!c.fileResolvedRef.current && c.loadData && c.selectedFileIdx >= 0 && !isLibrarySource(c.item)) {
        c.fileResolvedRef.current = true;
        c.onFileResolvedRef.current?.(c.loadData.info_hash, c.selectedFileIdx);
      }
    } },
    onCanPlay: (event: VideoEvent) => {
      if (hlsMode) return;
      const video = event.currentTarget;
      if (video !== videoRef.current) return;
      video.volume = c.volume;
      video.muted = c.isMuted;
      c.setIsBuffering(false);
      if (resumePlaybackRef.current && video.paused) {
        video.play().catch((err: DOMException) => {
          if (video !== videoRef.current || err.name === "AbortError") return;
          if(err.name==="NotAllowedError"){resumePlaybackRef.current=false;loadStartedAtRef.current=null;c.setIsPlaying(false);c.setNeedsPlaybackGesture(true);diagnosticEvent(diagnostic,"playback.gesture_required");return;}
          c.setPlaybackError("Impossible de reprendre la lecture. Cliquez sur Lecture pour réessayer.");
        });
      }
    },
    onError: (event: VideoEvent) => {
      if (hlsMode) return;
      if (event.currentTarget !== videoRef.current) return;
      const mediaError = event.currentTarget.error;
      diagnosticEvent(diagnostic,"playback.media_error",{error_code:String(mediaError?.code||0),ready_state:event.currentTarget.readyState,network_state:event.currentTarget.networkState,video_codec:videoMeta?.video_codec||"",position:playbackOffset+event.currentTarget.currentTime});
      c.setIsBuffering(false);
      c.setIsPlaying(false);
      c.setPlaybackError(mediaErrorMessage(mediaError?.code, videoMeta?.video_codec));
    },
    onEnded: (event: VideoEvent) => {
      if (event.currentTarget !== videoRef.current) return;
      if (c.onPlaybackFailure && endedEarly(playbackOffset + event.currentTarget.currentTime, totalDuration)) {
        c.reportFailure("La lecture de cette source s’est interrompue avant la fin de l’épisode.","premature_end");
        return;
      }
      if (c.onNextEpisode) c.setNextCountdown(NEXT_EPISODE_DELAY);
    },
  };
}
