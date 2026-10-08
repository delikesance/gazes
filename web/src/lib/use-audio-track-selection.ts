"use client";
import { useEffect, type RefObject } from "react";
import type { VideoMetadata } from "@/types/api";
import { preferredAudioTrack } from "./media-tracks";

interface AudioTrackSelectionInput {
  meta: VideoMetadata | null;
  fileIndex: number;
  selected: number;
  /** True once the viewer picked a track: the preferred-language default stops applying. */
  manualRef: RefObject<boolean>;
  /** Runs on every pick, even of the current track (closes the options sheet). */
  onPick: () => void;
  /** Switches playback to another track. */
  apply: (track: number) => void;
}

/** Audio track picks, plus the automatic switch to the preferred language once the file's tracks are known. */
export function useAudioTrackSelection({ meta, fileIndex, selected, manualRef, onPick, apply }: AudioTrackSelectionInput) {
  const select = (trackIdx: number, manual = true) => {
    if (manual) manualRef.current = true;
    onPick();
    if (trackIdx === selected) return;
    apply(trackIdx);
  };

  useEffect(() => {
    if (!meta || fileIndex < 0) return;
    const preferred = preferredAudioTrack(meta.audio_tracks || [], selected, manualRef.current);
    if (preferred !== selected) select(preferred, false);
  }, [meta, fileIndex, selected]); // eslint-disable-line react-hooks/exhaustive-deps

  return select;
}
