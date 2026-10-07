// Pure player logic (no React, no DOM): VideoPlayerModal and its hooks call these, scripts/player-state.test.mjs covers them.
import type { EpisodeSource, LoadTorrentResponse, SubtitleTrack, TorrentItem } from '../types/api';
import type { SubtitleStyle } from './ass-style';
import { DEFAULT_SUBTITLE_STYLE, SUBTITLE_LIFTS, SUBTITLE_SCALES } from './ass-style';
import type { AmbilightSettings } from '../components/PlayerOptionsModal';

export const NEXT_EPISODE_DELAY = 5;

export function formatTime(seconds: number): string {
  if (!seconds || isNaN(seconds) || seconds < 0) return "00:00";
  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) {
    return `${h}:${m < 10 ? "0" : ""}${m}:${s < 10 ? "0" : ""}${s}`;
  }
  return `${m < 10 ? "0" : ""}${m}:${s < 10 ? "0" : ""}${s}`;
}

// Viewer preferences, persisted in localStorage as raw strings.
export const SUBTITLE_STYLE_KEY = "gazes-subtitle-style";
export const AMBILIGHT_KEY = "gazes-ambilight";
export const RATE_KEY = "gazes-playback-rate";
export const AMBILIGHT_DEFAULT: AmbilightSettings = { on: true, level: "medium", dim: true };
export const PLAYBACK_RATES = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];

/** Unknown or malformed values fall back to the defaults, field by field. */
export function parseSubtitleStyle(raw: string | null): SubtitleStyle {
  try {
    const value = raw ? JSON.parse(raw) : null;
    return {
      scale: SUBTITLE_SCALES.includes(value?.scale) ? value.scale : DEFAULT_SUBTITLE_STYLE.scale,
      lift: SUBTITLE_LIFTS.includes(value?.lift) ? value.lift : DEFAULT_SUBTITLE_STYLE.lift,
    };
  } catch {
    return DEFAULT_SUBTITLE_STYLE;
  }
}

export function parseAmbilight(raw: string | null): AmbilightSettings {
  try {
    return raw ? { ...AMBILIGHT_DEFAULT, ...JSON.parse(raw) } : AMBILIGHT_DEFAULT;
  } catch {
    return AMBILIGHT_DEFAULT;
  }
}

export function parseRate(raw: string | null): number {
  const rate = Number(raw);
  return PLAYBACK_RATES.includes(rate) ? rate : 1;
}

/** Next rate in PLAYBACK_RATES, clamped at both ends. */
export function stepRate(rate: number, direction: 1 | -1): number {
  const index = PLAYBACK_RATES.indexOf(rate);
  return PLAYBACK_RATES[Math.max(0, Math.min(PLAYBACK_RATES.length - 1, index + direction))];
}

// Only PGS can be rendered as a bitmap; other bitmap codecs (VobSub, DVB, XSUB) cannot be converted.
const UNSUPPORTED_SUBTITLE_CODECS = new Set(["dvd_subtitle", "dvb_subtitle", "xsub"]);
export const isBitmapSubtitle = (track?: SubtitleTrack) => track?.codec === "hdmv_pgs_subtitle";
export function textSubtitleTracks(tracks?: SubtitleTrack[]): SubtitleTrack[] {
  return (tracks ?? []).filter((track) => !UNSUPPORTED_SUBTITLE_CODECS.has(track.codec));
}

// Partial tracks (forced signs, SDH, dubbing credits, commentary) are never a good default.
const PARTIAL_SUBTITLE = /\b(forced|sdh|cc|dubbing|dubtitle|signs?|songs?|commentary|karaoke)\b/i;
/** Default subtitle: a full French track, else any full track, else the default one. */
export function pickDefaultSubtitle(tracks: SubtitleTrack[]): SubtitleTrack {
  const full = tracks.filter((track) => !track.is_forced && !PARTIAL_SUBTITLE.test(track.title));
  const pool = full.length ? full : tracks;
  const french = (track: SubtitleTrack) => /^(fre|fra|fr)$/i.test(track.language) || /french|français|vostfr/i.test(track.title);
  return pool.find(french) ?? pool.find((track) => track.is_default) ?? pool[0];
}

export type PlayerItem = TorrentItem | null;

/** A library copy is a plain file on the server: no swarm, no file matching. */
export function libraryLoad(item: PlayerItem): LoadTorrentResponse | null {
  const library = item && "library" in item ? (item as EpisodeSource).library : undefined;
  return library ? { info_hash: library.stream_id, files: [{ index: 0, path: "episode.mkv", length: 0, is_video: true, mime_type: "video/x-matroska" }], main_video_index: 0 } : null;
}

/** Torrent files are matched once metadata arrives (-1 until then); a library copy is file 0. */
export function initialFileIndex(item: PlayerItem): number {
  return item && !libraryLoad(item) ? -1 : 0;
}

export function clampSeek(target: number, totalDuration: number): number {
  if (target < 0) return 0;
  if (totalDuration > 0 && target > totalDuration) return totalDuration;
  return target;
}

/** Arrow-key volume: 5 % steps from the audible level, snapped to the step grid. */
export function stepVolume(volume: number, muted: boolean, up: boolean): number {
  return Math.max(0, Math.min(1, Math.round(((muted ? 0 : volume) + (up ? 0.05 : -0.05)) * 20) / 20));
}

/** Played share of the timeline, in percent. */
export function progressPercent(position: number, totalDuration: number): number {
  return Math.min(100, Math.max(0, (position / totalDuration) * 100));
}

/**
 * The buffered bar, in percent of the timeline: from the stream start (playbackOffset) to the end of
 * the buffered range that holds the playhead. Ranges are in video time, the timeline in file time.
 */
export function bufferedSpan(ranges: [number, number][], now: number, playbackOffset: number, totalDuration: number): { left: number; width: number } {
  let end = now;
  for (const [start, stop] of ranges) {
    if (start <= now + 0.5 && stop >= now) end = Math.max(end, stop);
  }
  const left = Math.min(100, Math.max(0, (playbackOffset / totalDuration) * 100));
  const right = Math.min(100, Math.max(left, ((playbackOffset + end) / totalDuration) * 100));
  return { left, width: right - left };
}

/** The scrubber slider: arrows seek 5 s, Page keys 30 s, Home/End jump to the edges. null = not a slider key. */
export function scrubberTarget(key: string, position: number, totalDuration: number): number | null {
  const step = ({ ArrowLeft: -5, ArrowDown: -5, ArrowRight: 5, ArrowUp: 5, PageDown: -30, PageUp: 30 } as Record<string, number>)[key];
  let target: number;
  if (step !== undefined) target = position + step;
  else if (key === "Home") target = 0;
  else if (key === "End") target = totalDuration;
  else return null;
  return Math.max(0, Math.min(totalDuration, target));
}

export type PlayerShortcut =
  | { action: "toggle-play" | "fullscreen" | "mute" | "pip" | "next-episode" | "skip" }
  | { action: "seek"; delta: number }
  | { action: "volume"; up: boolean }
  | { action: "rate"; direction: 1 | -1 };

export interface ShortcutKey { code: string; ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean; shiftKey?: boolean }
export interface ShortcutContext { hasNextEpisode: boolean; canPip: boolean; hasSkip: boolean }

/** Window-level player shortcuts. null = not ours: the event keeps its default behavior. */
export function playerShortcut(key: ShortcutKey, context: ShortcutContext): PlayerShortcut | null {
  const modified = !!(key.ctrlKey || key.metaKey || key.altKey);
  switch (key.code) {
    case "Space":
    case "KeyK":
      return { action: "toggle-play" };
    case "ArrowLeft":
    case "KeyJ":
      return { action: "seek", delta: -10 };
    case "ArrowRight":
    case "KeyL":
      return { action: "seek", delta: 10 };
    case "ArrowUp":
    case "ArrowDown":
      return { action: "volume", up: key.code === "ArrowUp" };
    case "KeyN":
      return modified || !context.hasNextEpisode ? null : { action: "next-episode" };
    case "KeyF":
      return { action: "fullscreen" };
    case "KeyM":
      return { action: "mute" };
    case "KeyP":
      return modified || !context.canPip ? null : { action: "pip" };
    case "Comma":
    case "Period":
      return !key.shiftKey || modified ? null : { action: "rate", direction: key.code === "Period" ? 1 : -1 };
    case "KeyS":
      return modified || !context.hasSkip ? null : { action: "skip" };
  }
  return null;
}

/** Identifies the file an AniSkip lookup or preview frame belongs to; "" when it is not known yet. */
export function episodeFileKey(seasonId: number, episodeNumber: number | undefined, infoHash: string | undefined, fileIndex: number): string {
  return seasonId > 0 && episodeNumber && infoHash !== undefined && fileIndex >= 0 ? `${seasonId}:${episodeNumber}:${infoHash}:${fileIndex}` : "";
}

export interface PlaybackFailure { reason: string; code: string }

/** Why the pack cannot be played as this episode: nothing matches, or several files do. */
export function fileSelectionFailure(candidates: number): PlaybackFailure {
  return {
    reason: candidates === 0 ? "L’épisode demandé est introuvable dans ce pack." : "L’épisode demandé n’est pas identifié sans ambiguïté dans ce pack.",
    code: "episode_missing_or_ambiguous",
  };
}

export interface WatchdogTimeouts { metadata: number; startup: number; stall: number; max: number }

/** Metadata phase: the swarm has as long as it shows activity, never more than `max`. */
export function metadataTimedOut(now: number, startedAt: number, lastActivity: number, timeouts: WatchdogTimeouts): boolean {
  return now - lastActivity >= timeouts.metadata || now - startedAt >= timeouts.max;
}

/** Stream phase: a source that never starts, or stops both playing and downloading, has failed. */
export function streamWatchdog(
  state: { now: number; startedAt: number; lastActivity: number; lastProgressAt: number; hasStarted: boolean },
  timeouts: WatchdogTimeouts,
): PlaybackFailure | null {
  const { now, startedAt, lastActivity, lastProgressAt, hasStarted } = state;
  if (!hasStarted) {
    return now - lastActivity >= timeouts.startup || now - startedAt >= timeouts.max
      ? { reason: "La lecture ne démarre pas sur cette source.", code: "startup_timeout" } : null;
  }
  return now - lastProgressAt >= timeouts.stall && now - lastActivity >= timeouts.stall
    ? { reason: "Cette source ne fournit plus de vidéo.", code: "swarm_stall" } : null;
}

export const isHevc = (codec?: string) => {
  const value = codec?.toLowerCase();
  return value === "hevc" || value === "h265";
};

/** The overlay text for a <video> error: MediaError 3/4 are decode/format failures. */
export function mediaErrorMessage(code: number | undefined, videoCodec?: string): string {
  if (code === 3 || code === 4) {
    return isHevc(videoCodec)
      ? "Cette vidéo H.265/HEVC ne peut pas être décodée dans ce navigateur ou sur cet appareil. Essayez une source H.264/AVC ou un navigateur compatible HEVC."
      : "Impossible de décoder cette vidéo. Essayez une autre source ou un navigateur compatible avec son codec.";
  }
  return "La lecture a été interrompue. Réessayez ou choisissez une autre source.";
}

/** Ended this far before the known duration: the stream broke, it did not finish. */
export function endedEarly(position: number, totalDuration: number): boolean {
  return totalDuration > 0 && position < totalDuration - 2;
}
