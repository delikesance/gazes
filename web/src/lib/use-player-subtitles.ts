"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import type { VideoMetadata } from "@/types/api";
import { diagnosticEvent, type PlaybackDiagnostic } from "./diagnostics";
import { MAX_SUBTITLE_FILE_BYTES, subtitleFileToAss } from "./subtitle-file";
import { pickDefaultSubtitle, textSubtitleTracks } from "./player-state";

/** Which subtitle shows: an embedded track, a file the viewer picked, or none; plus the renderer's error toast. */
export function usePlayerSubtitles(diagnostic: PlaybackDiagnostic | undefined, t: (text: string) => string) {
  const [selectedSubTrack, setSelectedSubTrack] = useState<number | null>(null);
  // A subtitle file the viewer picked: ASS text rendered in place of the embedded tracks.
  const [localSubtitle, setLocalSubtitle] = useState<{ name: string; ass: string } | null>(null);
  const [subtitleFileError, setSubtitleFileError] = useState<string | null>(null);
  const [subtitleError, setSubtitleError] = useState<{ message: string; code?: string } | null>(null);
  const [toastHovered, setToastHovered] = useState(false);
  // Set once the viewer (or the automatic default) chose: later metadata must not override it.
  const choiceMadeRef = useRef(false);

  const handleSubtitleError = useCallback((message: string | null, code?: string) => {
    setSubtitleError(message ? { message, code } : null);
    if (message) diagnosticEvent(diagnostic, "playback.subtitle_failed", { error_code: code || "unknown" });
  }, [diagnostic]);

  // The subtitle error is informational: it fades out on its own.
  useEffect(() => {
    if (!subtitleError || toastHovered) return;
    const timer = setTimeout(() => setSubtitleError(null), 8000);
    return () => clearTimeout(timer);
  }, [subtitleError, toastHovered]);

  /** New metadata picks the default subtitle once, unless the viewer already chose one. */
  const offerTracks = useCallback((meta: VideoMetadata) => {
    const tracks = textSubtitleTracks(meta.subtitle_tracks);
    if (!choiceMadeRef.current && tracks.length) {
      choiceMadeRef.current = true;
      setSelectedSubTrack(pickDefaultSubtitle(tracks).index);
    }
  }, []);

  /** A new source or file: the next metadata picks a default again. */
  const resetChoice = useCallback(() => { choiceMadeRef.current = false; }, []);

  const selectTrack = useCallback((trackIdx: number | null) => {
    choiceMadeRef.current = true;
    setLocalSubtitle(null);
    setSelectedSubTrack(trackIdx);
  }, []);

  /** Resolves true once the file replaced the embedded tracks; false leaves an error to show. */
  const pickFile = useCallback(async (file: File) => {
    setSubtitleFileError(null);
    if (file.size > MAX_SUBTITLE_FILE_BYTES) { setSubtitleFileError(t("Fichier trop volumineux.")); return false; }
    const ass = subtitleFileToAss(file.name, await file.text().catch(() => ""));
    if (!ass) { setSubtitleFileError(t("Fichier de sous-titres illisible (formats : .srt, .vtt, .ass).")); return false; }
    choiceMadeRef.current = true;
    setLocalSubtitle({ name: file.name, ass });
    setSelectedSubTrack(null);
    return true;
  }, [t]);

  return {
    selectedSubTrack, setSelectedSubTrack, localSubtitle, subtitleFileError,
    subtitleError, setSubtitleError, setToastHovered, handleSubtitleError,
    offerTracks, resetChoice, selectTrack, pickFile,
  };
}
