"use client";
import { useWatchParty } from "@/lib/use-watch-party";
import { PlayerStartup } from "./PlayerStartup";
import { diagnosticEvent, type PlaybackDiagnostic } from "@/lib/diagnostics";
import { useI18n } from "@/lib/i18n";
import { mediaTrackLabel, preferredAudioTrack } from "@/lib/media-tracks";
import { copyText } from "@/lib/clipboard";

import { SubtitleRenderer } from "./SubtitleRenderer";
import { ErrorAlert } from "./ErrorAlert";
import { PlayerDebugPanel, useDebugMode, type DebugAttempt } from "./PlayerDebugPanel";
import { PlayerEpisodePicker } from "./PlayerEpisodePicker";
import { PlayerFailover, type FailoverInfo } from "./PlayerFailover";
import { PlayerOptionsModal, type AmbilightSettings, type PlayerOptionsTab } from "./PlayerOptionsModal";
import { PLAYBACK_TIMEOUTS } from "@/lib/playback-sources";
import { episodeFile, episodeCandidates } from "@/lib/episode-file";
import { HlsPlaybackController } from "@/lib/hls-playback";
import { activeSkipSegment, mergeSkipSegments, needsAniSkip, shiftSegments, skipAction, type SkipSegment } from "@/lib/skip-segments";
import { SkipSegmentButton } from "./SkipSegmentButton";
import { usePlaybackEngine } from "@/lib/use-playback-engine";
import type { EpisodeInfo, EpisodeSource } from "@/types/api";
import { createPortal } from "react-dom";
import React, { useEffect, useState, useRef, useCallback } from "react";
import { TorrentItem, LoadTorrentResponse, SwarmStats, FileInfo, VideoMetadata, SubtitleTrack } from "@/types/api";
import { requestEpisodePreview, getSkipTimes, loadTorrent, getTorrentStats, getStreamUrl, getSubtitleUrl, fetchVideoMetadata, formatBytes } from "@/lib/api";
import {
  ArrowLeft,
  X,
  Play,
  Pause,
  Maximize,
  Minimize,
  Volume2,
  VolumeX,
  Volume1,
  RotateCcw,
  RotateCw,
  Copy,
  Check,
  AlertCircle,
  MessageSquare,
  SkipForward,
  SkipBack,
  Layers,
  Loader2,
  Settings,
  List,
  Sun,
  Image as ImageIcon,
  Link2,
  Users,
} from "lucide-react";

interface VideoPlayerModalProps {
  initialPaused?: boolean;
  onPlaybackIntent?: (playing: boolean) => void;
  item: TorrentItem | null;
  diagnostic?: PlaybackDiagnostic;
  pageMode?: boolean;
  initialTime?: number;
  onProgress?: (position:number, duration:number, tracks?:{audioLang?:string; subLang?:string})=>void;
  onPlaybackFailure?: (failure: { reason: string; position: number }) => void;
  onVideoMetadata?: (metadata: VideoMetadata) => void;
  onClose: () => void;
  animeTitle?: string;
  episodeNumber?: number;
  totalEpisodes?: number;
  onNextEpisode?: () => void;
  onPrevEpisode?: () => void;
  onChangeSource?: () => void;
  sourcePicker?: React.ReactNode;
  episodes?: EpisodeInfo[];
  /** Shown for episodes the metadata provider has no still for. */
  fallbackThumbnail?: string;
  onSelectEpisode?: (episode: number) => void;
  failover?: FailoverInfo;
  debugAttempt?: DebugAttempt;
  /** Called once, when a torrent file (not a library copy) first plays. */
  onFileResolved?: (infoHash: string, fileIndex: number) => void;
}

const AMBILIGHT_KEY = "gazes-ambilight";
const AMBILIGHT_DEFAULT: AmbilightSettings = { on: true, level: "medium", dim: true };

function loadAmbilight(): AmbilightSettings {
  try {
    const raw = typeof window !== "undefined" ? window.localStorage.getItem(AMBILIGHT_KEY) : null;
    return raw ? { ...AMBILIGHT_DEFAULT, ...JSON.parse(raw) } : AMBILIGHT_DEFAULT;
  } catch {
    return AMBILIGHT_DEFAULT;
  }
}

// Only PGS can be rendered as a bitmap; other bitmap codecs (VobSub, DVB, XSUB) cannot be converted.
const UNSUPPORTED_SUBTITLE_CODECS = new Set(["dvd_subtitle", "dvb_subtitle", "xsub"]);
const isBitmapSubtitle = (track?: SubtitleTrack) => track?.codec === "hdmv_pgs_subtitle";
function textSubtitleTracks(tracks?: SubtitleTrack[]): SubtitleTrack[] {
  return (tracks ?? []).filter((track) => !UNSUPPORTED_SUBTITLE_CODECS.has(track.codec));
}

// Partial tracks (forced signs, SDH, dubbing credits, commentary) are never a good default.
const PARTIAL_SUBTITLE = /\b(forced|sdh|cc|dubbing|dubtitle|signs?|songs?|commentary|karaoke)\b/i;
/** Default subtitle: a full French track, else any full track, else the default one. */
function pickDefaultSubtitle(tracks: SubtitleTrack[]): SubtitleTrack {
  const full = tracks.filter((track) => !track.is_forced && !PARTIAL_SUBTITLE.test(track.title));
  const pool = full.length ? full : tracks;
  const french = (track: SubtitleTrack) => /^(fre|fra|fr)$/i.test(track.language) || /french|français|vostfr/i.test(track.title);
  return pool.find(french) ?? pool.find((track) => track.is_default) ?? pool[0];
}

function formatTime(seconds: number): string {
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

const NEXT_EPISODE_DELAY = 5;

export const VideoPlayerModal: React.FC<VideoPlayerModalProps> = ({
  item,
  onFileResolved,
  onClose,
  animeTitle,
  episodeNumber,
  onNextEpisode,
  onPrevEpisode,
  onChangeSource,
  sourcePicker,
  episodes,
  fallbackThumbnail,
  onSelectEpisode,
  failover,
  debugAttempt,
  pageMode = false,
  initialTime = 0,
  onPlaybackFailure,
  onVideoMetadata,
  onProgress,
  diagnostic,
  initialPaused = false,
  onPlaybackIntent,
}) => {
  const { t, locale } = useI18n();
  const debugMode = useDebugMode();
  const engine = usePlaybackEngine();
  const hlsMode = engine === "hls";
  const hlsController = useRef<HlsPlaybackController | null>(null);
  const [subtitleOrigin, setSubtitleOrigin] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loadData, setLoadData] = useState<LoadTorrentResponse | null>(null);
  const [fileSearch, setFileSearch] = useState("");
  const [needsFileSelection, setNeedsFileSelection] = useState(false);
  const [selectedFileIdx, setSelectedFileIdx] = useState<number>(0);
  const [stats, setStats] = useState<SwarmStats | null>(null);
  const [videoMeta, setVideoMeta] = useState<VideoMetadata | null>(null);
  const [copied, setCopied] = useState(false);
  const [forceRemux, setForceRemux] = useState(initialTime > 0);
  const [isBuffering, setIsBuffering] = useState(false);
  const [playbackError, setPlaybackError] = useState<string | null>(null);

  // Audio and Subtitle Tracks
  const [selectedAudioTrack, setSelectedAudioTrack] = useState<number>(0);
  const [selectedSubTrack, setSelectedSubTrack] = useState<number | null>(null);
  const [started, setStarted] = useState(false);
  const [optionsTab, setOptionsTab] = useState<PlayerOptionsTab | null>(null);
  const [showEpisodes, setShowEpisodes] = useState(false);
  const [ambilight, setAmbilight] = useState<AmbilightSettings>(loadAmbilight);

  // Player controls state
  const [isPlaying, setIsPlaying] = useState(false);
  const [needsPlaybackGesture,setNeedsPlaybackGesture]=useState(false);
  const [timeOffset, setTimeOffset] = useState(initialTime);
  const playbackOffset = hlsMode ? 0 : timeOffset;
  const [volume, setVolume] = useState(1);
  const [isMuted, setIsMuted] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showControls, setShowControls] = useState(true);
  const [hoverTime, setHoverTime] = useState<number | null>(null);
  const [hoverPosition, setHoverPosition] = useState<number>(0);

  const containerRef = useRef<HTMLDivElement>(null);
  const dockRef = useRef<HTMLDivElement>(null);
  const ambientRef = useRef<HTMLCanvasElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const [subtitleError, setSubtitleError] = useState<{ message: string; code?: string } | null>(null);
  const [toastHovered, setToastHovered] = useState(false);
  const handleSubtitleError = useCallback((message: string | null, code?: string) => {
    setSubtitleError(message ? { message, code } : null);
    if (message) diagnosticEvent(diagnostic, "playback.subtitle_failed", { error_code: code || "unknown" });
  }, [diagnostic]);
  const resumePlaybackRef = useRef(!initialPaused);
  const failureReportedRef = useRef(false);
  const hasStartedRef = useRef(false);
  const loadStartedAtRef = useRef<number | null>(null);
  const fileResolvedRef = useRef(false);
  const onFileResolvedRef = useRef(onFileResolved);
  useEffect(() => { onFileResolvedRef.current = onFileResolved; }, [onFileResolved]);
  const lastProgressRef = useRef({ time: 0, at: 0 });
  const subtitleSelectionRef = useRef(false);
  const audioSelectionRef = useRef(false);
  const progressBarRef = useRef<HTMLDivElement>(null);
  const playedBarRef = useRef<HTMLDivElement>(null);
  const bufferedBarRef = useRef<HTMLDivElement>(null);
  const knobRef = useRef<HTMLDivElement>(null);
  const timeDisplayRef = useRef<HTMLSpanElement>(null);
  const currentTimeRef = useRef<number>(0);
  const hideControlsTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const pollIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const itemHash = item?.info_hash || item?.id || null;
  const [prevItemHash, setPrevItemHash] = useState<string | null>(itemHash);
  // Seconds left before the next episode starts on its own; null while idle or cancelled.
  const [nextCountdown, setNextCountdown] = useState<number | null>(null);

  if (itemHash !== prevItemHash) {
    setPrevItemHash(itemHash);
    setNextCountdown(null); // the HLS player stays mounted across episodes: a pending countdown belongs to the old one
    setLoading(true);
    setError(null);
    setStats(null);
    setVideoMeta(null);
    setTimeOffset(initialTime);
    setForceRemux(false);
    setSelectedAudioTrack(0);
    setSelectedSubTrack(null);
  }

  // The subtitle error is informational: it fades out on its own.
  useEffect(() => {
    if (!subtitleError || toastHovered) return;
    const timer = setTimeout(() => setSubtitleError(null), 8000);
    return () => clearTimeout(timer);
  }, [subtitleError, toastHovered]);

  const totalDuration = videoMeta?.duration_sec || loadData?.main_video_metadata?.duration_sec || 0;

  // Timeline: positions here are `playbackOffset + video.currentTime`, the value handleSeek takes.
  // Legacy: the remux starts at timeOffset, so that sum is the file time (subtitles: no correction).
  // HLS: playbackOffset is 0 and fragments/subtitles are rebased by -timeline_origin server side,
  // so chapter times (raw file time) are shifted by -origin. AniSkip times already count from the
  // first frame, so they are not shifted.
  const skipSeasonId = Number(diagnostic?.season_id);
  const skipDuration = Math.round(videoMeta?.duration_sec || 0);
  const skipKey = skipSeasonId > 0 && episodeNumber && skipDuration > 0 && loadData && selectedFileIdx >= 0
    ? `${skipSeasonId}:${episodeNumber}:${loadData.info_hash}:${selectedFileIdx}` : "";
  const skipOrigin = hlsMode ? subtitleOrigin : 0;
  const chapterSkips = React.useMemo(
    () => shiftSegments(videoMeta?.skip_segments ?? [], skipOrigin),
    [videoMeta?.skip_segments, skipOrigin],
  );
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
  const skipSegments = React.useMemo(
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
  }, [skipSegments, playbackOffset, updateActiveSkip]);
  const shownSkip = activeSkip && skipSegments.includes(activeSkip) ? activeSkip : null;
  const shownSkipAction = shownSkip ? skipAction(shownSkip, totalDuration, !!onNextEpisode) : "seek";

  // Direct DOM updates for smooth timeline
  const updateProgressDisplay = useCallback(
    (timeSec: number) => {
      const cur = playbackOffset + timeSec;
      if (timeDisplayRef.current) {
        timeDisplayRef.current.textContent = formatTime(cur);
      }
      if (totalDuration > 0) {
        const pct = Math.min(100, Math.max(0, (cur / totalDuration) * 100));
        if (playedBarRef.current) {
          playedBarRef.current.style.width = `${pct}%`;
        }
        if (knobRef.current) {
          knobRef.current.style.left = `${pct}%`;
        }
      }
    },
    [playbackOffset, totalDuration]
  );

  // The white bar: how far ahead of the playhead the browser already holds data.
  const updateBuffered = useCallback(() => {
    const video = videoRef.current;
    const bar = bufferedBarRef.current;
    if (!video || !bar || totalDuration <= 0) return;
    const now = video.currentTime;
    let end = now;
    for (let i = 0; i < video.buffered.length; i++) {
      if (video.buffered.start(i) <= now + 0.5 && video.buffered.end(i) >= now) end = Math.max(end, video.buffered.end(i));
    }
    const start = Math.min(100, Math.max(0, (playbackOffset / totalDuration) * 100));
    const stop = Math.min(100, Math.max(start, ((playbackOffset + end) / totalDuration) * 100));
    bar.style.left = `${start}%`;
    bar.style.width = `${stop - start}%`;
  }, [playbackOffset, totalDuration]);

  useEffect(() => () => {
    if (hideControlsTimeoutRef.current) clearTimeout(hideControlsTimeoutRef.current);
  }, []);

  useEffect(()=>{
    if(videoMeta)diagnosticEvent(diagnostic,"playback.tracks",{video_codec:videoMeta.video_codec,audio_codec:videoMeta.audio_tracks?.find(t=>t.index===selectedAudioTrack)?.codec||'',audio_track:selectedAudioTrack,subtitle_track:selectedSubTrack??-1,duration:videoMeta.duration_sec});
  },[diagnostic,videoMeta,selectedAudioTrack,selectedSubTrack]);

  const reportFailure = useCallback((reason: string, error_code="playback_failed") => {
    if (!onPlaybackFailure || failureReportedRef.current) return;
    failureReportedRef.current = true;
    const position = hlsController.current?.position ?? (playbackOffset + (videoRef.current?.currentTime ?? currentTimeRef.current));
    diagnosticEvent(diagnostic,"playback.failed",{reason,error_code,position});
    onPlaybackFailure({ reason, position });
  }, [onPlaybackFailure, playbackOffset]);

  useEffect(() => {
    if (error) reportFailure(error,"torrent_metadata_failed");
    else if (needsFileSelection) {
      const none = !!loadData && !!item && episodeCandidates(loadData.files, item as EpisodeSource).length === 0;
      reportFailure(none ? "L’épisode demandé est introuvable dans ce pack." : "L’épisode demandé n’est pas identifié sans ambiguïté dans ce pack.","episode_missing_or_ambiguous");
    }
    else if (playbackError) reportFailure(playbackError,"media_error");
  }, [error, needsFileSelection, playbackError, reportFailure, loadData, item]);

  // Metadata phase: wait as long as the swarm is alive (a connected peer or incoming bytes).
  useEffect(() => {
    if (!onPlaybackFailure || !item || !loading || (item as EpisodeSource).library) return;
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
      if (now - lastActivity >= PLAYBACK_TIMEOUTS.metadata || now - startedAt >= PLAYBACK_TIMEOUTS.max) {
        reportFailure("Cette source ne fournit pas ses métadonnées à temps.", "metadata_timeout");
      }
    }, 2000);
    return () => clearInterval(check);
  }, [loading, item, onPlaybackFailure, reportFailure, diagnostic]);

  // Actual time advancement proves playback. Buffering can occur without an
  // error event, so a dead swarm must not hold this source indefinitely.
  useEffect(() => {
    if (hlsMode || !onPlaybackFailure || loading || needsFileSelection) return;
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
      if (!hasStartedRef.current) {
        if (now-lastActivity >= PLAYBACK_TIMEOUTS.startup || now-startedAt >= PLAYBACK_TIMEOUTS.max) reportFailure("La lecture ne démarre pas sur cette source.","startup_timeout");
      } else if (now-lastProgressRef.current.at >= PLAYBACK_TIMEOUTS.stall && now-lastActivity >= PLAYBACK_TIMEOUTS.stall) {
        reportFailure("Cette source ne fournit plus de vidéo.","swarm_stall");
      }
    }, 1000);
    return () => clearInterval(poll);
  }, [hlsMode, loading, needsFileSelection, onPlaybackFailure, reportFailure, timeOffset, selectedAudioTrack, item, diagnostic]);

  // Auto-hide controls timer
  const triggerShowControls = useCallback(() => {
    setShowControls(true);
    if (hideControlsTimeoutRef.current) clearTimeout(hideControlsTimeoutRef.current);
    if (isPlaying) {
      hideControlsTimeoutRef.current = setTimeout(() => {
        setShowControls(false);
      }, 2500);
    }
  }, [isPlaying]);

  const togglePlay = useCallback(() => {
    if (!videoRef.current) return;
    if (hlsMode && hlsController.current) {
      if (resumePlaybackRef.current) hlsController.current.pause(); else hlsController.current.play();
      triggerShowControls();
      return;
    }
    if (!resumePlaybackRef.current || (videoRef.current.paused && !isBuffering)) {
      resumePlaybackRef.current = true;
      lastProgressRef.current.at = Date.now();
      videoRef.current.play().catch((err:DOMException) => {
 if(err.name==="AbortError")return;
 if(err.name==="NotAllowedError"){resumePlaybackRef.current=false;setIsPlaying(false);setNeedsPlaybackGesture(true);diagnosticEvent(diagnostic,"playback.gesture_required");return;}
 setPlaybackError("Impossible de reprendre la lecture. Cliquez sur Lecture pour réessayer.");
 });
      setIsPlaying(true);
    } else {
      resumePlaybackRef.current = false;
      videoRef.current.pause();
      setIsPlaying(false);
    }
    triggerShowControls();
  }, [triggerShowControls, isBuffering, hlsMode]);

  const partySeekRef = useRef<(seconds: number) => void>(undefined);
  const handleSeek = useCallback((targetSec: number) => {
    diagnosticEvent(diagnostic,"playback.seek",{position:targetSec});
    let finalSec = targetSec;
    if (finalSec < 0) finalSec = 0;
    if (totalDuration > 0 && finalSec > totalDuration) finalSec = totalDuration;
    setNextCountdown(null); // seeking back after the end means staying on this episode
    partySeekRef.current?.(finalSec);
    if (hlsMode) { hlsController.current?.seek(finalSec); updateProgressDisplay(finalSec); triggerShowControls(); return; }

    hasStartedRef.current = false;
    lastProgressRef.current = { time: 0, at: 0 };
    setIsBuffering(true);
    setForceRemux(true);
    setTimeOffset(finalSec);
    currentTimeRef.current = 0;
    setNeedsFileSelection(false);
    updateProgressDisplay(0);

    triggerShowControls();
  }, [hlsMode, totalDuration, updateProgressDisplay, triggerShowControls]);

  const runSkip = useCallback(() => {
    if (!shownSkip) return;
    if (shownSkipAction === "next-episode") onNextEpisode?.();
    else handleSeek(shownSkip.end);
  }, [shownSkip, shownSkipAction, onNextEpisode, handleSeek]);

  const party = useWatchParty({
    isPlaying,
    getPosition: () => hlsController.current?.position ?? (playbackOffset + (videoRef.current?.currentTime ?? currentTimeRef.current)),
    togglePlay,
    seek: handleSeek,
  });
  const partySend = party.send;
  useEffect(() => { partySeekRef.current = (seconds) => partySend("seek", seconds); }, [partySend]);
  const [partyCopied, setPartyCopied] = useState(false);
  const copyPartyLink = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(party.invite());
      setPartyCopied(true);
      window.setTimeout(() => setPartyCopied(false), 2000);
    } catch { /* clipboard unavailable (insecure context) */ }
  }, [party]);
  // Playing again (replay, a party member resuming) cancels the countdown.
  if (isPlaying && nextCountdown !== null) setNextCountdown(null);
  useEffect(() => {
    if (nextCountdown === null) return;
    const timer = window.setTimeout(() => {
      if (nextCountdown <= 1) { setNextCountdown(null); onNextEpisode?.(); } else setNextCountdown(nextCountdown - 1);
    }, 1000);
    return () => window.clearTimeout(timer);
  }, [nextCountdown, onNextEpisode]);
  const [linkCopied, setLinkCopied] = useState(false);
  const copyTimeLink = useCallback(async () => {
    const position = Math.floor(hlsController.current?.position ?? (playbackOffset + (videoRef.current?.currentTime ?? currentTimeRef.current)));
    const url = new URL(window.location.href);
    url.search = position > 0 ? `?t=${position}` : "";
    url.hash = "";
    try {
      await navigator.clipboard.writeText(url.toString());
      setLinkCopied(true);
      window.setTimeout(() => setLinkCopied(false), 2000);
    } catch { /* clipboard unavailable (insecure context): nothing to copy */ }
  }, [playbackOffset]);

  const toggleMute = useCallback(() => {
    if (!videoRef.current) return;
    const nextMuted = !isMuted;
    videoRef.current.muted = nextMuted;
    setIsMuted(nextMuted);
    triggerShowControls();
  }, [isMuted, triggerShowControls]);

  const toggleFullscreen = useCallback(() => {
    if (!containerRef.current) return;
    if (!document.fullscreenElement) {
      containerRef.current.requestFullscreen().catch(() => {});
    } else {
      document.exitFullscreen().catch(() => {});
    }
    triggerShowControls();
  }, [triggerShowControls]);

  // 1. Load Torrent Metadata
  useEffect(() => {
    if (!item) return;

    resumePlaybackRef.current = !initialPaused;
    failureReportedRef.current = false;
    hasStartedRef.current = false;
    loadStartedAtRef.current = performance.now();
    fileResolvedRef.current = false;
    setPlaybackError(null);
    subtitleSelectionRef.current = false;
    audioSelectionRef.current = false;
    let isMounted = true;
    const controller = new AbortController();
    setLoadData(null);
    setSelectedFileIdx(-1);
    setNeedsFileSelection(false);
    setFileSearch("");
    currentTimeRef.current = 0;

    const library = "library" in item ? (item as EpisodeSource).library : undefined;
    if (library) {
      // Library copies are plain files on the server: no swarm, no file matching.
      setLoadData({ info_hash: library.stream_id, files: [{ index: 0, path: "episode.mkv", length: 0, is_video: true, mime_type: "video/x-matroska" }], main_video_index: 0 });
      setSelectedFileIdx(0);
      setNeedsFileSelection(false);
      setLoading(false);
      return () => {
        isMounted = false;
        diagnosticEvent(diagnostic,"playback.abandoned",{position:playbackOffset+currentTimeRef.current});
        controller.abort();
      };
    }

    loadTorrent(item.magnet_uri, {metadataOnly: true, signal:controller.signal,diagnostic})
      .then((data) => {
        if (!isMounted) return;
        setLoadData(data);
        const isEpisode = "episode_number" in item && typeof item.episode_number === "number" && item.episode_number > 0;
        const matched = isEpisode ? episodeFile(data.files,item as EpisodeSource) : data.main_video_index;
        const initialIdx = matched === null ? -1 : matched;
        setNeedsFileSelection(matched === null);
        const selected=data.files.find(f=>f.index===initialIdx);
        diagnosticEvent(diagnostic,selected?'playback.file_selected':'playback.file_rejected',{file_index:initialIdx,file_path:selected?.path||'',file_count:data.files.length,reason:selected?'episode_match':'episode_missing_or_ambiguous'});
        setSelectedFileIdx(initialIdx);
        if (data.main_video_metadata && initialIdx === data.main_video_index) {
          setVideoMeta(data.main_video_metadata);
          if (textSubtitleTracks(data.main_video_metadata.subtitle_tracks).length > 0) {
            subtitleSelectionRef.current = true;
            setSelectedSubTrack(pickDefaultSubtitle(textSubtitleTracks(data.main_video_metadata.subtitle_tracks)).index);
          }

        }
        setLoading(false);
      })
      .catch((err) => {
        if (!isMounted) return;
        setError(err.message || "Failed to load torrent metadata from swarm");
        setLoading(false);
      });

    return () => {
      isMounted = false;
      diagnosticEvent(diagnostic,"playback.abandoned",{position:playbackOffset+currentTimeRef.current});
      controller.abort();
      if (pollIntervalRef.current) clearInterval(pollIntervalRef.current);
    };
  }, [item]);

  // 2. Poll Live Swarm Stats
  useEffect(() => {
    if (!loadData || !loadData.info_hash || (item && "library" in item && (item as EpisodeSource).library)) return;

    const fetchStats = () => {
      getTorrentStats(loadData.info_hash,diagnostic)
        .then((s) => {setStats(s);diagnosticEvent(diagnostic,"playback.swarm",{seeders:s.active_seeders,download_speed:s.download_rate_bps,buffer_percent:s.progress_pct});})
        .catch(() => {});
    };

    fetchStats();
    pollIntervalRef.current = setInterval(fetchStats, 5000);

    return () => {
      if (pollIntervalRef.current) clearInterval(pollIntervalRef.current);
    };
  }, [loadData]);

  // Probe only the selected episode, one request at a time. Cancel an old
  // request when its source/file changes instead of accumulating blocked reads.
  useEffect(() => {
    if (hlsMode || !loadData || selectedFileIdx < 0) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const probe = async () => {
      try {
        const m = await fetchVideoMetadata(loadData.info_hash, selectedFileIdx, controller.signal,diagnostic);
        if (controller.signal.aborted) return;
        if (m && (m.duration_sec > 0 || m.video_codec)) {
          setVideoMeta(m);
          if (!subtitleSelectionRef.current && textSubtitleTracks(m.subtitle_tracks).length) {
            const preferred = pickDefaultSubtitle(textSubtitleTracks(m.subtitle_tracks));
            setSelectedSubTrack(preferred.index);
            subtitleSelectionRef.current = true;
          }
          if (m.duration_sec > 0) return;
        }
      } catch { if (controller.signal.aborted) return; }
      timer = setTimeout(probe, 1500);
    };
    void probe();
    return () => { controller.abort(); if (timer) clearTimeout(timer); };
  }, [hlsMode, loadData, selectedFileIdx]);

  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!containerRef.current && document.fullscreenElement === containerRef.current);
      triggerShowControls();
    };
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, [triggerShowControls]);

  // 4. Global Keyboard Shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('[role="dialog"]') || ["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes((e.target as HTMLElement).tagName)) {
        return;
      }

      const cur = playbackOffset + currentTimeRef.current;

      switch (e.code) {
        case "Space":
        case "KeyK":
          e.preventDefault();
          togglePlay();
          break;
        case "ArrowLeft":
        case "KeyJ":
          e.preventDefault();
          handleSeek(cur - 10);
          break;
        case "ArrowRight":
        case "KeyL":
          e.preventDefault();
          handleSeek(cur + 10);
          break;
        case "ArrowUp":
        case "ArrowDown": {
          e.preventDefault();
          const next = Math.max(0, Math.min(1, Math.round(((isMuted ? 0 : volume) + (e.code === "ArrowUp" ? 0.05 : -0.05)) * 20) / 20));
          setVolume(next);
          if (videoRef.current) { videoRef.current.volume = next; videoRef.current.muted = next === 0; }
          setIsMuted(next === 0);
          break;
        }
        case "KeyN":
          if (e.ctrlKey || e.metaKey || e.altKey || !onNextEpisode) break;
          e.preventDefault();
          onNextEpisode();
          break;
        case "KeyF":
          e.preventDefault();
          toggleFullscreen();
          break;
        case "KeyM":
          e.preventDefault();
          toggleMute();
          break;
        case "KeyS":
          if (e.ctrlKey || e.metaKey || e.altKey || !shownSkip) break;
          e.preventDefault();
          runSkip();
          break;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [playbackOffset, togglePlay, handleSeek, toggleFullscreen, toggleMute, shownSkip, runSkip, isMuted, volume, onNextEpisode]);


  const matchingFiles = loadData && item && "episode_number" in item ? episodeCandidates(loadData.files,item as EpisodeSource) : [];
  const selectionFiles = (matchingFiles.length ? matchingFiles : loadData?.files.filter(file=>file.is_video) || []).filter(file=>file.path.toLowerCase().includes(fileSearch.toLowerCase()));
  const currentFile: FileInfo | undefined = loadData?.files[selectedFileIdx];
  const streamUrl = loadData && selectedFileIdx >= 0
    ? hlsMode ? `hls:${loadData.info_hash}:${selectedFileIdx}:${selectedAudioTrack}` : getStreamUrl(loadData.info_hash, selectedFileIdx, forceRemux, timeOffset, selectedAudioTrack,diagnostic)
    : "";

  useEffect(() => {
    setPlaybackError(null);
    // An unsupported HEVC track may still play its AAC audio without a media error.
    // Check both common HEVC profiles rather than waiting for onError alone.
    const codec = hlsMode ? undefined : videoMeta?.video_codec?.toLowerCase();
    if ((codec === "hevc" || codec === "h265") && videoRef.current &&
        videoRef.current.readyState < HTMLMediaElement.HAVE_CURRENT_DATA &&
        !videoRef.current.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"') &&
        !videoRef.current.canPlayType('video/mp4; codecs="hvc1.2.4.L123.B0"')) {
      setPlaybackError("Ce navigateur ne prend pas en charge la vidéo H.265/HEVC. Choisissez une source H.264/AVC ou un navigateur compatible HEVC.");
    }
  }, [hlsMode, streamUrl, videoMeta?.video_codec, loading, needsFileSelection]);

  const hlsResumePosition = useRef(initialTime);
  useEffect(() => {
    if (!hlsMode || loading || needsFileSelection || !loadData || selectedFileIdx < 0 || !videoRef.current) return;
    const video = videoRef.current;
    const controller = new HlsPlaybackController(video, {
      state: state => {
        currentTimeRef.current = state.position;
        resumePlaybackRef.current = state.playing;
        onPlaybackIntent?.(state.playing);
        setIsPlaying(state.playing);
        setIsBuffering(["preparing", "seeking", "buffering"].includes(state.phase));
        video.dataset.playbackPhase = state.phase;
        video.dataset.seekGeneration = String(state.generation);
        if (state.phase === "ready") setStarted(true);
      },
      metadata: (metadata, origin) => { setVideoMeta(metadata); setSubtitleOrigin(origin); },
      gesture: () => setNeedsPlaybackGesture(true),
      error: message => { setPlaybackError(message); },
    }, diagnostic, !resumePlaybackRef.current);
    hlsController.current = controller;
    const position = hlsResumePosition.current;
    void controller.open(loadData.info_hash, selectedFileIdx, selectedAudioTrack, position);
    return () => { hlsResumePosition.current = video.currentTime || position; controller.dispose(); if (hlsController.current === controller) hlsController.current = null; };
  }, [hlsMode, loadData, selectedFileIdx, selectedAudioTrack, loading, needsFileSelection]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { hlsResumePosition.current = initialTime; }, [itemHash, initialTime]);

  const subtitleBitmap = isBitmapSubtitle(videoMeta?.subtitle_tracks?.find((track) => track.index === selectedSubTrack));
  const subtitleUrl =
    loadData && selectedSubTrack !== null
      ? getSubtitleUrl(loadData.info_hash, selectedFileIdx, selectedSubTrack, subtitleBitmap ? "sup" : "ass",diagnostic) + (hlsMode ? `&timeline_origin=${subtitleOrigin}` : "")
      : "";

  // Mount a fresh decoder for each source and restore the user's audio settings.
  useEffect(() => {
    const video = videoRef.current;
    if (video) video.volume = volume;
  }, [streamUrl, volume, loading, needsFileSelection]);

  const handleAudioTrackSelect = (trackIdx: number, manual = true) => {
    if (manual) audioSelectionRef.current = true;
    setOptionsTab(null);
    if (trackIdx === selectedAudioTrack) return;
    if (hlsMode) { hlsResumePosition.current = videoRef.current?.currentTime ?? currentTimeRef.current; setSelectedAudioTrack(trackIdx); return; }
    const currentAbsoluteTime = playbackOffset + (videoRef.current?.currentTime ?? currentTimeRef.current);
    hasStartedRef.current = false;
    lastProgressRef.current = { time: 0, at: 0 };
    setForceRemux(true);
    setSelectedAudioTrack(trackIdx);
    setIsBuffering(true);
    setTimeOffset(currentAbsoluteTime);
    currentTimeRef.current = 0;

    triggerShowControls();
  };

  useEffect(() => {
    if (!videoMeta || selectedFileIdx < 0) return;
    const preferred = preferredAudioTrack(videoMeta.audio_tracks || [], selectedAudioTrack, audioSelectionRef.current);
    if (preferred !== selectedAudioTrack) handleAudioTrackSelect(preferred, false);
  }, [videoMeta, selectedFileIdx, selectedAudioTrack]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!videoMeta || subtitleSelectionRef.current) return;
    const tracks = textSubtitleTracks(videoMeta.subtitle_tracks);
    if (tracks.length) { subtitleSelectionRef.current = true; setSelectedSubTrack(pickDefaultSubtitle(tracks).index); }
  }, [videoMeta]);

  useEffect(() => {
    if (selectedFileIdx >= 0 && videoMeta?.probe_status === 'complete') onVideoMetadata?.(videoMeta);
  }, [videoMeta, selectedFileIdx, onVideoMetadata]);

  // Both engines end up here: once the file's duration is known, have the backend cut the episode's preview frame.
  const previewRequested = useRef("");
  const previewAlive = useRef(true);
  const [previewSaved, setPreviewSaved] = useState(false);
  useEffect(() => { previewAlive.current = true; return () => { previewAlive.current = false; }; }, []);
  useEffect(() => {
    const seasonId = Number(diagnostic?.season_id);
    if (!(seasonId > 0) || !episodeNumber || !loadData || selectedFileIdx < 0 || !videoMeta?.duration_sec) return;
    if (episodes?.find(e => e.episode_number === episodeNumber)?.thumbnail) return;
    const key = `${seasonId}:${episodeNumber}:${loadData.info_hash}:${selectedFileIdx}`;
    if (previewRequested.current === key) return;
    previewRequested.current = key;
    void requestEpisodePreview(seasonId, episodeNumber, loadData.info_hash, selectedFileIdx, videoMeta.duration_sec, () => previewAlive.current).then(created => {
      if (!created || !previewAlive.current) return;
      setPreviewSaved(true);
      setTimeout(() => { if (previewAlive.current) setPreviewSaved(false); }, 4000);
    });
  }, [videoMeta, diagnostic?.season_id, episodeNumber, loadData, selectedFileIdx, episodes]);

  const handleSubtitleTrackSelect = (trackIdx: number | null) => {
    subtitleSelectionRef.current = true;
    setSelectedSubTrack(trackIdx);
    setOptionsTab(null);
    triggerShowControls();
  };

  const handleProgressBarClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!progressBarRef.current || totalDuration <= 0) return;
    const rect = progressBarRef.current.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    handleSeek(ratio * totalDuration);
  };

  const handleProgressBarMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!progressBarRef.current || totalDuration <= 0) return;
    const rect = progressBarRef.current.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    setHoverPosition(ratio * 100);
    setHoverTime(ratio * totalDuration);
  };

  const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseFloat(e.target.value);
    setVolume(val);
    if (videoRef.current) {
      videoRef.current.volume = val;
      videoRef.current.muted = val === 0;
    }
    setIsMuted(val === 0);
    triggerShowControls();
  };

  const handleCopyMagnet = () => {
    if (!item) return;
    void copyText(item.magnet_uri);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const updateAmbilight = (patch: Partial<AmbilightSettings>) => {
    setAmbilight((current) => {
      const next = { ...current, ...patch };
      try { window.localStorage.setItem(AMBILIGHT_KEY, JSON.stringify(next)); } catch { /* storage unavailable */ }
      return next;
    });
  };

  // Ambilight: paint a tiny copy of the current frame (cover-fit to the stage); CSS blurs and fades it.
  useEffect(() => {
    const canvas = ambientRef.current;
    const video = videoRef.current;
    const box = containerRef.current;
    if (!ambilight.on || isFullscreen || !canvas || !video || !box) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    let frame = 0;
    let last = 0;
    let stopped = false;
    const draw = () => {
      if (video.readyState < 2 || !video.videoWidth) return;
      const width = 64;
      const height = Math.max(8, Math.round((width * (box.clientHeight || 1)) / (box.clientWidth || 1)));
      if (canvas.width !== width || canvas.height !== height) { canvas.width = width; canvas.height = height; }
      const scale = Math.max(width / video.videoWidth, height / video.videoHeight);
      const w = video.videoWidth * scale;
      const h = video.videoHeight * scale;
      try { ctx.drawImage(video, (width - w) / 2, (height - h) / 2, w, h); } catch { /* frame unavailable */ }
    };
    const loop = (now: number) => {
      if (stopped) return;
      if (now - last >= 100 && !document.hidden && !video.paused) { last = now; draw(); }
      frame = requestAnimationFrame(loop);
    };
    const events = ["loadeddata", "seeked", "pause", "playing"] as const;
    events.forEach((name) => video.addEventListener(name, draw));
    draw();
    frame = requestAnimationFrame(loop);
    return () => {
      stopped = true;
      cancelAnimationFrame(frame);
      events.forEach((name) => video.removeEventListener(name, draw));
    };
  }, [ambilight.on, isFullscreen, streamUrl, loading, error, needsFileSelection, item]);

  // While the dock is visible, shrink the subtitle layer (not the video) so captions stay above it.
  useEffect(() => {
    const stage = containerRef.current;
    const dock = dockRef.current;
    if (!stage || !dock) return;
    const update = () => {
      const height = stage.clientHeight;
      const reserve = dock.offsetHeight + (parseFloat(getComputedStyle(dock).bottom) || 0) + 12;
      stage.style.setProperty("--sub-scale", String(height > 0 ? Math.max(0.5, (height - reserve) / height) : 1));
      stage.style.setProperty("--dock-reserve", `${reserve}px`);
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(stage);
    observer.observe(dock);
    return () => observer.disconnect();
  }, [loading, error, needsFileSelection, item]);

  if (!item) return null;

  // Fullscreen only paints descendants of the video box. Keep the heading
  // inside that layer while fullscreen, without remounting the video itself.
  const topBar = (
    <div onMouseMove={triggerShowControls} className={`player-heading player-topbar${!showControls?" watch-heading-hidden":""}`}>
      <div className="flex min-w-0 items-center gap-2.5">
        <button
          aria-label={t(pageMode?"Voir les saisons":"Fermer le lecteur")}
          onClick={onClose}
          className="player-frost player-pill player-pill--icon shrink-0"
        >
          {pageMode ? <ArrowLeft className="h-5 w-5" aria-hidden="true" /> : <X className="h-5 w-5" aria-hidden="true" />}
        </button>
        <div className="player-frost player-pill player-episode-pill min-w-0">
          {episodeNumber && <span className="player-chip player-chip--solid">EP {episodeNumber}</span>}
          {previewSaved && <span role="status" className="player-chip player-chip--solid player-chip--preview"><ImageIcon size={13} aria-hidden="true" /><Check size={13} aria-hidden="true" />{t("Aperçu enregistré")}</span>}
          <h2 className="min-w-0 truncate text-[13px] font-medium" title={animeTitle || item.title}>
            {animeTitle || item.anime_details?.display_title || item.title}
          </h2>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2.5">
        {stats && (
          <div className="player-frost player-pill hidden !gap-2 sm:inline-flex" role="status">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" aria-hidden="true" />
            <span className="font-mono text-xs">{stats.active_seeders} {t("Seeders")} · {formatBytes(stats.download_rate_bps)}/s</span>
          </div>
        )}
        {onChangeSource && (
          <button onClick={onChangeSource} aria-label={t("Changer de source pour cet épisode")} title={t("Changer de source pour cet épisode")} className="player-frost player-pill player-pill--collapse">
            <Layers className="h-4 w-4" /><span className="player-label">{t("Sources")}</span>
          </button>
        )}
      </div>
    </div>
  );

  return (
    <div
      className={pageMode?"watch-player fixed inset-0 z-50 bg-black":"fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-sm p-2 sm:p-6 animate-in fade-in duration-150"}
      onClick={pageMode?undefined:onClose}
    >
      <div
        className={pageMode?"watch-player-shell":"relative flex flex-col w-full max-w-5xl max-h-[95vh] rounded-[28px] border border-zinc-800 bg-zinc-950 overflow-hidden shadow-2xl"}
        onClick={(e) => e.stopPropagation()}
      >
        {isFullscreen && containerRef.current ? createPortal(topBar, containerRef.current) : topBar}

        {/* Player & Content Area */}
        <div className="player-content flex-1 overflow-y-auto">
          {loading && !hlsMode ? (
            <PlayerStartup stage="connect" image={fallbackThumbnail} title={animeTitle} episode={episodeNumber} />
          ) : needsFileSelection && !hlsMode ? (
            <div className="p-6 pt-24 space-y-4">
              <p>{onPlaybackFailure ? t("Cet épisode ne peut pas être identifié dans ce pack. Essai de la source suivante…") : t("Choisissez le fichier correspondant à l’épisode {episode}. Aucun fichier n’a été lancé automatiquement.", {episode:episodeNumber ?? "—"})}</p>
              {!onPlaybackFailure && <>
              <input aria-label={t("Rechercher un fichier")} placeholder={t("Rechercher un fichier…")} value={fileSearch} onChange={event=>setFileSearch(event.target.value)} className="w-full bg-zinc-900 border border-zinc-700 p-3 rounded-full" />
              <p>{selectionFiles.length} {" "}{t("fichiers")}{matchingFiles.length ? t(" correspondant à cet épisode") : t(" vidéo")}</p>
              {selectionFiles.slice(0,50).map(file => <button key={file.index} className="block p-3 bg-zinc-900 rounded-2xl text-left w-full" onClick={() => { setSelectedFileIdx(file.index); setNeedsFileSelection(false); }}>{file.path}</button>)}
              {selectionFiles.length>50 && <p>{t("Affichage des 50 premiers fichiers. Affinez la recherche.")}</p>}
              </>}
            </div>
          ) : error && !hlsMode ? (
            <div className="flex flex-col items-center justify-center py-20 px-6 text-center space-y-3">
              <AlertCircle className="h-8 w-8 text-zinc-500" />
              <p className="text-xs text-zinc-200">{t(error)}</p>
              <p className="text-[11px] text-zinc-400 max-w-md">
                {t("The torrent swarm may have no active seeders or tracker connection timed out.")}</p>
            </div>
          ) : (
            <div>
              {/* Video Player Box */}
              <div
                ref={containerRef}
                onMouseMove={triggerShowControls}
                onMouseEnter={triggerShowControls}
                data-controls={showControls}
                className={`player-video-box relative w-full bg-black select-none group overflow-hidden flex flex-col${pageMode?"":" aspect-video"}`}
              >
                <div className="relative flex-1 min-h-0 overflow-hidden" data-player-stage>
                {ambilight.on && !isFullscreen && (
                  <canvas ref={ambientRef} aria-hidden="true" className="player-ambient" data-level={ambilight.level} data-dim={ambilight.dim && !isPlaying ? "true" : "false"} />
                )}
                <video
                  key={hlsMode ? "hls-video" : streamUrl}
                  muted={isMuted}
                  ref={videoRef}
                  src={engine && !hlsMode ? streamUrl : undefined}
                  playsInline
                  crossOrigin="anonymous"
                  onTimeUpdate={(event) => {
                  if (hlsMode && event.currentTarget.dataset.playbackPhase === "preparing") return;
                  if(!hasStartedRef.current&&event.currentTarget===videoRef.current&&event.currentTarget.currentTime>lastProgressRef.current.time&&event.currentTarget.videoWidth>0){const startedAt=loadStartedAtRef.current;loadStartedAtRef.current=null;diagnosticEvent(diagnostic,"playback.started",{position:playbackOffset+event.currentTarget.currentTime,width:event.currentTarget.videoWidth,height:event.currentTarget.videoHeight,...(startedAt!==null?{startup_ms:Math.round(performance.now()-startedAt)}:{})});}
                    if (event.currentTarget === videoRef.current) {
                      const time = videoRef.current.currentTime;
                      if (time > lastProgressRef.current.time && videoRef.current.videoWidth > 0) {
                        hasStartedRef.current = true;
                        lastProgressRef.current = { time, at: Date.now() };
                      }
                      currentTimeRef.current = time;
                      onProgress?.(playbackOffset + time, totalDuration, {audioLang:videoMeta?.audio_tracks?.find(track=>track.index===selectedAudioTrack)?.language, subLang:selectedSubTrack===null?"":videoMeta?.subtitle_tracks?.find(track=>track.index===selectedSubTrack)?.language});
                      updateProgressDisplay(videoRef.current.currentTime);
                      updateBuffered();
                      updateActiveSkip(playbackOffset + time);
                    }
                  }}
                  onPlay={(event) => { if (event.currentTarget === videoRef.current) {resumePlaybackRef.current=true;setNeedsPlaybackGesture(false);setIsPlaying(true);} }}
                  onPause={(event) => { if (event.currentTarget === videoRef.current) setIsPlaying(false); }}
                  onWaiting={(event) => { if (event.currentTarget === videoRef.current) {setIsBuffering(true);diagnosticEvent(diagnostic,"playback.buffering",{position:playbackOffset+event.currentTarget.currentTime,ready_state:event.currentTarget.readyState});} }}
                  onProgress={() => updateBuffered()}
                  onPlaying={(event) => { if (event.currentTarget === videoRef.current) {
                    setIsBuffering(false); setStarted(true);
                    if (!fileResolvedRef.current && loadData && selectedFileIdx >= 0 && !(item && "library" in item && (item as EpisodeSource).library)) {
                      fileResolvedRef.current = true;
                      onFileResolvedRef.current?.(loadData.info_hash, selectedFileIdx);
                    }
                  } }}
                  onCanPlay={(event) => {
                    if (hlsMode) return;
                    const video = event.currentTarget;
                    if (video !== videoRef.current) return;
                    video.volume = volume;
                    video.muted = isMuted;
                    setIsBuffering(false);
                    if (resumePlaybackRef.current && video.paused) {
                      video.play().catch((err: DOMException) => {
                        if (video !== videoRef.current || err.name === "AbortError") return;
                        if(err.name==="NotAllowedError"){resumePlaybackRef.current=false;setIsPlaying(false);setNeedsPlaybackGesture(true);diagnosticEvent(diagnostic,"playback.gesture_required");return;}
                        setPlaybackError("Impossible de reprendre la lecture. Cliquez sur Lecture pour réessayer.");
                      });
                    }
                  }}
                  onError={(event) => {
                    if (hlsMode) return;
                    if (event.currentTarget !== videoRef.current) return;
                    const mediaError = event.currentTarget.error;
                    diagnosticEvent(diagnostic,"playback.media_error",{error_code:String(mediaError?.code||0),ready_state:event.currentTarget.readyState,network_state:event.currentTarget.networkState,video_codec:videoMeta?.video_codec||"",position:playbackOffset+event.currentTarget.currentTime});
                    setIsBuffering(false);
                    setIsPlaying(false);
                    if (mediaError?.code === 3 || mediaError?.code === 4) {
                      const codec = videoMeta?.video_codec?.toLowerCase();
                      setPlaybackError(codec === "hevc" || codec === "h265"
                        ? "Cette vidéo H.265/HEVC ne peut pas être décodée dans ce navigateur ou sur cet appareil. Essayez une source H.264/AVC ou un navigateur compatible HEVC."
                        : "Impossible de décoder cette vidéo. Essayez une autre source ou un navigateur compatible avec son codec.");
                    } else {
                      setPlaybackError("La lecture a été interrompue. Réessayez ou choisissez une autre source.");
                    }
                  }}
                  onEnded={(event) => {
                    if (event.currentTarget !== videoRef.current) return;
                    if (onPlaybackFailure && totalDuration > 0 && playbackOffset + event.currentTarget.currentTime < totalDuration - 2) {
                      reportFailure("La lecture de cette source s’est interrompue avant la fin de l’épisode.","premature_end");
                      return;
                    }
                    if (onNextEpisode) setNextCountdown(NEXT_EPISODE_DELAY);
                  }}
                  onClick={togglePlay}
                  className="absolute inset-0 h-full w-full object-contain cursor-pointer"
                >
                  {t("Your browser does not support HTML5 video playback.")}</video>
                <SubtitleRenderer
                  videoRef={videoRef}
                  streamKey={streamUrl}
                  url={subtitleUrl}
                  bitmap={subtitleBitmap}
                  timeOffset={playbackOffset}
                  onError={handleSubtitleError}
                />
                {debugMode && (
                  <PlayerDebugPanel
                    diagnostic={diagnostic} item={item} file={currentFile} fileIndex={selectedFileIdx} fileCount={loadData?.files.length}
                    meta={videoMeta} audioTrack={selectedAudioTrack} subtitleTrack={selectedSubTrack} remux={forceRemux}
                    timeOffset={playbackOffset} streamUrl={streamUrl} subtitleUrl={subtitleUrl} attempt={debugAttempt}
                  />
                )}
                {subtitleError && (
                  <div onMouseEnter={() => setToastHovered(true)} onMouseLeave={() => setToastHovered(false)} className="contents">
                    <ErrorAlert className="player-toast" message={subtitleError.message} code={subtitleError.code} reference={diagnostic?.playback_session_id} onClose={() => setSubtitleError(null)} />
                  </div>
                )}
                </div>
                {needsPlaybackGesture&&<div className="absolute inset-0 z-20 flex items-center justify-center pointer-events-none"><button className="pointer-events-auto flex items-center gap-2 rounded-full bg-zinc-900/90 px-6 py-4 text-white border border-zinc-700" onClick={togglePlay}><Play size={22} />{t("Lecture")}</button></div>}
                {playbackError && (
                  <div role="alert" className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-4 bg-black/90 p-6 text-center">
                    <p className="text-sm text-zinc-200">{t(playbackError)}</p>
                    <button className="rounded-full bg-white px-4 py-2 text-black" onClick={() => { setPlaybackError(null); videoRef.current?.load(); }}>{t("Réessayer")}</button>
                    {onChangeSource && <button className="underline" onClick={onChangeSource}>{t("Changer de source")}</button>}
                  </div>
                )}

                {/* First-byte wait: the stream can take several seconds to start, never leave a bare black frame */}
                {!started && !error && !needsPlaybackGesture && (
                  <PlayerStartup stage="stream" overlay image={fallbackThumbnail} title={animeTitle} episode={episodeNumber} />
                )}

                {/* Buffering Indicator */}
                {isBuffering && (
                  <div className="player-frost absolute right-6 top-24 z-20 flex items-center gap-2 rounded-full px-3.5 py-1.5 font-mono text-xs text-zinc-200">
                    <Loader2 className="h-3 w-3 animate-spin text-zinc-400" />
                    <span>{t("Buffering stream...")}</span>
                  </div>
                )}

                {shownSkip && (
                  <SkipSegmentButton segment={shownSkip} action={shownSkipAction} onSkip={runSkip} />
                )}

                {/* Floating control dock */}
                <div
                  ref={dockRef}
                  data-player-controls
                  className={`player-dock-wrap pointer-events-auto ${showControls ? "visible" : "invisible !pointer-events-none"}`}
                >
                  {showEpisodes && episodes && episodes.length > 0 && onSelectEpisode && (
                    <PlayerEpisodePicker
                      episodes={episodes}
                      fallbackThumbnail={fallbackThumbnail}
                      seasonId={Number(diagnostic?.season_id) || undefined}
                      currentEpisode={episodeNumber}
                      onClose={() => setShowEpisodes(false)}
                      onSelect={(number) => { setShowEpisodes(false); if (number !== episodeNumber) onSelectEpisode(number); }}
                    />
                  )}
                  <div className="player-frost flex flex-col gap-3 px-4 py-3.5 text-zinc-200 sm:gap-3.5 sm:px-[18px] sm:py-4" style={{ borderRadius: "var(--radius-dock, 32px)" }}>
                    {/* Scrubber */}
                    <div className="flex items-center gap-3.5 px-1.5">
                      <span ref={timeDisplayRef} className="min-w-11 font-mono text-xs text-zinc-50">{formatTime(playbackOffset)}</span>
                      <div
                        ref={progressBarRef}
                        onClick={handleProgressBarClick}
                        onMouseMove={handleProgressBarMouseMove}
                        onMouseLeave={() => setHoverTime(null)}
                        className="group/bar relative flex h-5 flex-1 cursor-pointer items-center"
                      >
                        <div className="relative h-1.5 w-full rounded-full bg-white/20 transition-all group-hover/bar:h-2">
                          <div ref={bufferedBarRef} className="absolute top-0 h-full rounded-full bg-white/55" style={{ left: "0%", width: "0%" }} />
                          <div ref={playedBarRef} className="absolute left-0 top-0 h-full rounded-full bg-[var(--accent)]" style={{ width: "0%" }} />
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
                        {totalDuration > 0 ? formatTime(totalDuration) : videoMeta?.formatted_duration || "--:--"}
                      </span>
                    </div>

                    {/* Controls Row */}
                    <div className="player-controls-row flex items-center justify-between gap-3 text-xs">
                      <div className="flex items-center gap-2">
                        <button onClick={togglePlay} className="player-pill player-pill--icon player-pill--solid" aria-label={isPlaying ? t("Pause (Space)") : t("Play (Space)")} title={isPlaying ? t("Pause (Space)") : t("Play (Space)")}>
                          {isPlaying ? <Pause className="h-5 w-5 fill-current" /> : <Play className="h-5 w-5 fill-current" />}
                        </button>
                        {onPrevEpisode && (
                          <button onClick={onPrevEpisode} className="player-pill player-pill--icon" aria-label={t("Previous Episode")} title={t("Previous Episode")}>
                            <SkipBack className="h-[18px] w-[18px]" />
                          </button>
                        )}
                        {onNextEpisode && (
                          <button onClick={onNextEpisode} className="player-pill player-pill--icon" aria-label={t("Next Episode")} title={t("Next Episode")}>
                            <SkipForward className="h-[18px] w-[18px]" />
                          </button>
                        )}
                        <button onClick={() => handleSeek(playbackOffset + currentTimeRef.current - 10)} className="player-pill player-pill--icon" aria-label={t("Rewind 10s (←)")} title={t("Rewind 10s (←)")}>
                          <RotateCcw className="h-[18px] w-[18px]" />
                        </button>
                        <button onClick={() => handleSeek(playbackOffset + currentTimeRef.current + 10)} className="player-pill player-pill--icon" aria-label={t("Forward 10s (→)")} title={t("Forward 10s (→)")}>
                          <RotateCw className="h-[18px] w-[18px]" />
                        </button>
                        {/* Volume */}
                        <div className="hidden items-center gap-3 rounded-full bg-white/[.06] pr-5 sm:flex">
                          <button onClick={toggleMute} className="player-pill player-pill--icon" aria-label={t(isMuted ? "Unmute (M)" : "Mute (M)")} title={t(isMuted ? "Unmute (M)" : "Mute (M)")}>
                            {isMuted || volume === 0 ? <VolumeX className="h-[18px] w-[18px]" /> : volume < 0.5 ? <Volume1 className="h-[18px] w-[18px]" /> : <Volume2 className="h-[18px] w-[18px]" />}
                          </button>
                          <input
                            aria-label={t("Volume")}
                            type="range"
                            min="0"
                            max="1"
                            step="0.05"
                            value={isMuted ? 0 : volume}
                            onChange={handleVolumeChange}
                            style={{ background: `linear-gradient(to right, var(--accent) ${(isMuted ? 0 : volume) * 100}%, rgb(255 255 255 / 0.2) ${(isMuted ? 0 : volume) * 100}%)` }}
                            className="h-1.5 w-20 cursor-pointer appearance-none rounded-full accent-white"
                          />
                        </div>
                      </div>

                      {/* Right: Episodes, Settings, Fullscreen */}
                      <div className="flex items-center gap-2">
                        {episodes && episodes.length > 0 && onSelectEpisode && (
                          <button
                            onClick={() => { setShowEpisodes(!showEpisodes); }}
                            data-active={showEpisodes}
                            aria-expanded={showEpisodes}
                            className="player-pill player-pill--collapse"
                            title={t("Épisodes")}
                          >
                            <List className="h-4 w-4" /><span className="player-label">{t("Épisodes")}</span>
                          </button>
                        )}
                        <button
                          onClick={() => { setShowEpisodes(false); setOptionsTab(optionsTab ? null : "audio"); }}
                          data-active={optionsTab !== null}
                          aria-haspopup="dialog"
                          className="player-pill player-pill--collapse"
                          title={t("Select Audio Track")}
                        >
                          <Settings className="h-4 w-4" /><span className="player-label">{t("Réglages")}</span>
                        </button>
                        <button onClick={copyPartyLink} className="player-pill player-pill--icon" data-active={party.room !== null} aria-live="polite" aria-label={t(partyCopied ? "Lien copié" : "Regarder ensemble : copier l’invitation")} title={t(partyCopied ? "Lien copié" : "Regarder ensemble : copier l’invitation")}>
                          <Users className="h-[18px] w-[18px]" />
                        </button>
                        <button onClick={copyTimeLink} className="player-pill player-pill--icon" aria-live="polite" aria-label={t(linkCopied ? "Lien copié" : "Copier le lien à cet instant")} title={t(linkCopied ? "Lien copié" : "Copier le lien à cet instant")}>
                          <Link2 className="h-[18px] w-[18px]" />
                        </button>
                        <button onClick={toggleFullscreen} className="player-pill player-pill--icon" aria-label={t("Toggle Fullscreen (F)")} title={t("Toggle Fullscreen (F)")}>
                          {isFullscreen ? <Minimize className="h-[18px] w-[18px]" /> : <Maximize className="h-[18px] w-[18px]" />}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>

                {nextCountdown !== null && (
                  <div className="next-episode-card" role="status" aria-live="polite">
                    <p>{t("Épisode suivant dans {seconds} s", { seconds: nextCountdown })}</p>
                    <div className="next-episode-actions">
                      <button type="button" className="clay clay-primary clay-sm" onClick={() => { setNextCountdown(null); onNextEpisode?.(); }}>{t("Lancer maintenant")}</button>
                      <button type="button" className="clay clay-secondary clay-sm" onClick={() => setNextCountdown(null)}>{t("Annuler")}</button>
                    </div>
                  </div>
                )}

                {failover && !started && <PlayerFailover info={failover} onChangeSource={onChangeSource} />}

                {optionsTab && (
                  <PlayerOptionsModal
                    tab={optionsTab}
                    onTabChange={setOptionsTab}
                    onClose={() => setOptionsTab(null)}
                    audioOptions={(videoMeta?.audio_tracks || []).map((track) => ({ index: track.index, title: track.title, label: mediaTrackLabel(track, videoMeta?.audio_tracks || [], locale) }))}
                    selectedAudio={selectedAudioTrack}
                    onSelectAudio={handleAudioTrackSelect}
                    subtitleOptions={textSubtitleTracks(videoMeta?.subtitle_tracks).map((track) => ({ index: track.index, title: track.title, label: mediaTrackLabel(track, textSubtitleTracks(videoMeta?.subtitle_tracks), locale) }))}
                    selectedSubtitle={selectedSubTrack}
                    onSelectSubtitle={handleSubtitleTrackSelect}
                    ambilight={ambilight}
                    onAmbilightChange={updateAmbilight}
                  />
                )}
              </div>

              {/* Swarm & Metadata Telemetry HUD */}
              <div className="player-telemetry border-b border-zinc-800 bg-zinc-900/20 p-4">
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                  {/* Seeders */}
                  <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("Seeders")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {stats?.active_seeders ?? item.seeders} <span className="text-zinc-500 font-normal">({stats?.total_peers ?? 0} {" "}{t("peers)")}</span>
                    </p>
                  </div>

                  {/* Download Speed */}
                  <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("Download Speed")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {stats ? `${formatBytes(stats.download_rate_bps)}/s` : t("Buffering...")}
                    </p>
                  </div>

                  {/* Duration */}
                  <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("Duration")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {totalDuration > 0 ? formatTime(totalDuration) : videoMeta?.formatted_duration || t("Detecting...")}
                    </p>
                  </div>

                  {/* File Size */}
                  <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("File Size")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {formatBytes(videoMeta?.total_bytes || currentFile?.length || item.size_bytes)}
                    </p>
                  </div>
                </div>
              </div>

              {/* File Selector & Controls */}
              <div className="player-advanced p-4 space-y-3">
                {!onPlaybackFailure && loadData && loadData.files.length > 1 && (
                  <div>
                    <label className="block text-xs text-zinc-400 mb-1 font-mono">
                      {t("Episode / File (")}{loadData.files.length} {t("items):")}</label>
                    <select
                      value={selectedFileIdx}
                      onChange={(e) => {
                        subtitleSelectionRef.current = false;
                        audioSelectionRef.current = false;
                        resumePlaybackRef.current = true;
                        setVideoMeta(null);
                        setSelectedFileIdx(parseInt(e.target.value, 10));
                        setTimeOffset(0);
                        currentTimeRef.current = 0;
                        setSelectedAudioTrack(0);
                        setSelectedSubTrack(null);
                        updateProgressDisplay(0);
                      }}
                      className="w-full rounded-full border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-200 focus:border-zinc-500 focus:outline-none font-mono"
                    >
                      {loadData.files.map((file) => (
                        <option key={file.index} value={file.index}>
                          {file.path} ({formatBytes(file.length)}) {file.is_video ? "🎬" : ""}
                        </option>
                      ))}
                    </select>
                  </div>
                )}

                <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                  <label className="flex items-center gap-2 text-xs text-zinc-400 cursor-pointer select-none">
                    <input
                      type="checkbox"
                      checked={forceRemux}
                      onChange={(e) => setForceRemux(e.target.checked)}
                      className="rounded border-zinc-700 bg-zinc-800 text-white focus:ring-0"
                    />
                    <span>{t("Force FFmpeg Remux (Stereo AAC Transcoding)")}</span>
                  </label>

                  <button
                    onClick={handleCopyMagnet}
                    className="flex items-center gap-1.5 rounded-full border border-zinc-800 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 hover:text-white transition-colors cursor-pointer"
                  >
                    {copied ? (
                      <>
                        <Check className="h-3.5 w-3.5 text-zinc-200" />
                        <span>{t("Magnet Copied")}</span>
                      </>
                    ) : (
                      <>
                        <Copy className="h-3.5 w-3.5 text-zinc-400" />
                        <span>{t("Copy Magnet Link")}</span>
                      </>
                    )}
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
      {sourcePicker && typeof document !== "undefined" && createPortal(sourcePicker, document.fullscreenElement || document.body)}
    </div>
  );
};
