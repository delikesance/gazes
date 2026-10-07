"use client";
import { useWatchParty } from "@/lib/use-watch-party";
import { PlayerStartup } from "./PlayerStartup";
import { diagnosticEvent, type PlaybackDiagnostic } from "@/lib/diagnostics";
import { useI18n } from "@/lib/i18n";
import { mediaTrackLabel, preferredAudioTrack } from "@/lib/media-tracks";

import { SubtitleRenderer } from "./SubtitleRenderer";
import { usePlayerPreferences } from "@/lib/use-player-preferences";
import { usePlayerSubtitles } from "@/lib/use-player-subtitles";
import {
  NEXT_EPISODE_DELAY, PLAYBACK_RATES,
  bufferedSpan, clampSeek, endedEarly, episodeFileKey, fileSelectionFailure, formatTime, initialFileIndex, isBitmapSubtitle, isHevc,
  libraryLoad, mediaErrorMessage, playerShortcut,
  progressPercent, stepVolume, textSubtitleTracks,
} from "@/lib/player-state";
import { ErrorAlert } from "./ErrorAlert";
import { PlayerDebugPanel, useDebugMode, type DebugAttempt } from "./PlayerDebugPanel";
import { PlayerEpisodePicker } from "./PlayerEpisodePicker";
import { PlayerFailover, type FailoverInfo } from "./PlayerFailover";
import { PlayerOptionsModal, type PlayerOptionsTab } from "./PlayerOptionsModal";
import { episodeFile, episodeCandidates } from "@/lib/episode-file";
import { HlsPlaybackController } from "@/lib/hls-playback";
import { useSkipSegments } from "@/lib/use-skip-segments";
import { usePlaybackWatchdog } from "@/lib/use-playback-watchdog";
import { useNextEpisodeCountdown } from "@/lib/use-next-episode-countdown";
import { SkipSegmentButton } from "./SkipSegmentButton";
import { PlayerTopBar } from "./PlayerTopBar";
import { PlayerTimeline } from "./PlayerTimeline";
import { PlayerControls } from "./PlayerControls";
import { PlayerNextEpisodeCard } from "./PlayerNextEpisodeCard";
import { PlayerDetails } from "./PlayerDetails";
import { PlayerFileSelection } from "./PlayerFileSelection";
import { usePlaybackEngine } from "@/lib/use-playback-engine";
import type { EpisodeInfo, EpisodeSource } from "@/types/api";
import { createPortal } from "react-dom";
import React, { useEffect, useState, useRef, useCallback } from "react";
import { TorrentItem, LoadTorrentResponse, SwarmStats, FileInfo, VideoMetadata } from "@/types/api";
import { requestEpisodePreview, loadTorrent, getTorrentStats, getStreamUrl, getSubtitleUrl, fetchVideoMetadata } from "@/lib/api";
import { AlertCircle, Loader2, Play } from "lucide-react";

interface VideoPlayerModalProps {
  /** Whether the viewer last paused; read when a source starts loading. */
  pausedIntent?: () => boolean;
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

export { PLAYBACK_RATES };

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
  pausedIntent,
  onPlaybackIntent,
}) => {
  const { t, locale } = useI18n();
  const debugMode = useDebugMode();
  const engine = usePlaybackEngine();
  const hlsMode = engine === "hls";
  const hlsController = useRef<HlsPlaybackController | null>(null);
  const [subtitleOrigin, setSubtitleOrigin] = useState(0);
  const [loading, setLoading] = useState(() => !libraryLoad(item));
  const [error, setError] = useState<string | null>(null);
  const [loadData, setLoadData] = useState<LoadTorrentResponse | null>(() => libraryLoad(item));
  const [needsFileSelection, setNeedsFileSelection] = useState(false);
  const [selectedFileIdx, setSelectedFileIdx] = useState<number>(() => initialFileIndex(item));
  const [stats, setStats] = useState<SwarmStats | null>(null);
  const [videoMeta, setVideoMeta] = useState<VideoMetadata | null>(null);
  const [forceRemux, setForceRemux] = useState(initialTime > 0);
  const [isBuffering, setIsBuffering] = useState(false);
  const [playbackError, setPlaybackError] = useState<string | null>(null);

  // Audio and Subtitle Tracks
  const [selectedAudioTrack, setSelectedAudioTrack] = useState<number>(0);
  const subtitles = usePlayerSubtitles(diagnostic, t);
  const { selectedSubTrack, setSelectedSubTrack, localSubtitle, subtitleError } = subtitles;
  const [started, setStarted] = useState(false);
  const [optionsTab, setOptionsTab] = useState<PlayerOptionsTab | null>(null);
  const [showEpisodes, setShowEpisodes] = useState(false);
  const { ambilight, updateAmbilight, subtitleStyle, updateSubtitleStyle, playbackRate, setPlaybackRate, stepPlaybackRate } = usePlayerPreferences();
  const [canPip] = useState(() => typeof document !== "undefined" && !!document.pictureInPictureEnabled);

  // Player controls state
  const [isPlaying, setIsPlaying] = useState(false);
  const [needsPlaybackGesture,setNeedsPlaybackGesture]=useState(false);
  const [timeOffset, setTimeOffset] = useState(initialTime);
  const playbackOffset = hlsMode ? 0 : timeOffset;
  const [volume, setVolume] = useState(1);
  const [isMuted, setIsMuted] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showControls, setShowControls] = useState(true);

  const containerRef = useRef<HTMLDivElement>(null);
  // Also kept in state: the fullscreen top bar is portaled into it during render.
  const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);
  const setContainer = useCallback((el: HTMLDivElement | null) => { containerRef.current = el; setContainerEl(el); }, []);
  const dockRef = useRef<HTMLDivElement>(null);
  const ambientRef = useRef<HTMLCanvasElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const resumePlaybackRef = useRef(true);
  const failureReportedRef = useRef(false);
  const hasStartedRef = useRef(false);
  const loadStartedAtRef = useRef<number | null>(null);
  const fileResolvedRef = useRef(false);
  const onFileResolvedRef = useRef(onFileResolved);
  useEffect(() => { onFileResolvedRef.current = onFileResolved; }, [onFileResolved]);
  const lastProgressRef = useRef({ time: 0, at: 0 });
  const audioSelectionRef = useRef(false);
  const { offerTracks: offerSubtitleTracks, resetChoice: resetSubtitleChoice } = subtitles;
  const receiveVideoMeta = useCallback((meta: VideoMetadata) => {
    setVideoMeta(meta);
    offerSubtitleTracks(meta);
  }, [offerSubtitleTracks]);
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
  const [nextCountdown, setNextCountdown] = useNextEpisodeCountdown(isPlaying, onNextEpisode);

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
    if (item) {
      setPlaybackError(null);
      setLoadData(libraryLoad(item));
      setSelectedFileIdx(initialFileIndex(item));
      setNeedsFileSelection(false);
      setLoading(!libraryLoad(item));
    }
  }

  const totalDuration = videoMeta?.duration_sec || loadData?.main_video_metadata?.duration_sec || 0;

  // Timeline: positions here are `playbackOffset + video.currentTime`, the value handleSeek takes.
  // Legacy: the remux starts at timeOffset, so that sum is the file time (subtitles: no correction).
  // HLS: playbackOffset is 0 and fragments/subtitles are rebased by -timeline_origin server side,
  // so chapter times (raw file time) are shifted by -origin. AniSkip times already count from the
  // first frame, so they are not shifted.
  const { shownSkip, shownSkipAction, updateActiveSkip } = useSkipSegments({
    seasonId: Number(diagnostic?.season_id), episodeNumber, infoHash: loadData?.info_hash, fileIndex: selectedFileIdx,
    durationSec: videoMeta?.duration_sec || 0, chapters: videoMeta?.skip_segments, origin: hlsMode ? subtitleOrigin : 0,
    totalDuration, hasNextEpisode: !!onNextEpisode, playbackOffset, currentTimeRef,
  });

  // Direct DOM updates for smooth timeline
  const updateProgressDisplay = useCallback(
    (timeSec: number) => {
      const cur = playbackOffset + timeSec;
      if (timeDisplayRef.current) {
        timeDisplayRef.current.textContent = formatTime(cur);
      }
      // Kept on the slider by hand: the timeline is repainted outside React for smoothness.
      progressBarRef.current?.setAttribute("aria-valuenow", String(Math.floor(cur)));
      progressBarRef.current?.setAttribute("aria-valuetext", formatTime(cur));
      if (totalDuration > 0) {
        const pct = progressPercent(cur, totalDuration);
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
    const ranges: [number, number][] = [];
    for (let i = 0; i < video.buffered.length; i++) ranges.push([video.buffered.start(i), video.buffered.end(i)]);
    const { left, width } = bufferedSpan(ranges, video.currentTime, playbackOffset, totalDuration);
    bar.style.left = `${left}%`;
    bar.style.width = `${width}%`;
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
      const failure = fileSelectionFailure(loadData && item ? episodeCandidates(loadData.files, item as EpisodeSource).length : 1);
      reportFailure(failure.reason, failure.code);
    }
    else if (playbackError) reportFailure(playbackError,"media_error");
  }, [error, needsFileSelection, playbackError, reportFailure, loadData, item]);

  usePlaybackWatchdog({
    item, diagnostic, enabled: !!onPlaybackFailure, hlsMode, loading, needsFileSelection, reportFailure,
    videoRef, resumePlaybackRef, hasStartedRef, lastProgressRef, timeOffset, audioTrack: selectedAudioTrack,
  });

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
 if(err.name==="NotAllowedError"){resumePlaybackRef.current=false;loadStartedAtRef.current=null;setIsPlaying(false);setNeedsPlaybackGesture(true);diagnosticEvent(diagnostic,"playback.gesture_required");return;}
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
    const finalSec = clampSeek(targetSec, totalDuration);
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
  }, [hlsMode, totalDuration, updateProgressDisplay, triggerShowControls, setNextCountdown]);

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

  // Safari/iOS only: opens the system AirPlay picker for the video element.
  const [canAirPlay] = useState(() => typeof window !== "undefined" && "WebKitPlaybackTargetAvailabilityEvent" in window);
  const showAirPlay = useCallback(() => {
    (videoRef.current as (HTMLVideoElement & { webkitShowPlaybackTargetPicker?: () => void }) | null)?.webkitShowPlaybackTargetPicker?.();
  }, []);

  const togglePip = useCallback(() => {
    const video = videoRef.current;
    if (!video || !document.pictureInPictureEnabled) return;
    if (document.pictureInPictureElement) document.exitPictureInPicture().catch(() => {});
    else video.requestPictureInPicture().catch(() => {});
    triggerShowControls();
  }, [triggerShowControls]);

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

    resumePlaybackRef.current = !pausedIntent?.();
    failureReportedRef.current = false;
    hasStartedRef.current = false;
    // Time to first frame only means something when nothing waits on the viewer (paused resume).
    loadStartedAtRef.current = pausedIntent?.() ? null : performance.now();
    fileResolvedRef.current = false;
    resetSubtitleChoice();
    audioSelectionRef.current = false;
    let isMounted = true;
    const controller = new AbortController();
    currentTimeRef.current = 0;

    // Library copies are plain files on the server: their load data is set on render, nothing to fetch.
    if (libraryLoad(item)) {
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
          receiveVideoMeta(data.main_video_metadata);

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
          receiveVideoMeta(m);
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

      const shortcut = playerShortcut(e, { hasNextEpisode: !!onNextEpisode, canPip, hasSkip: !!shownSkip });
      if (!shortcut) return;
      e.preventDefault();
      switch (shortcut.action) {
        case "toggle-play": togglePlay(); break;
        case "seek": handleSeek(playbackOffset + currentTimeRef.current + shortcut.delta); break;
        case "volume": {
          const next = stepVolume(volume, isMuted, shortcut.up);
          setVolume(next);
          if (videoRef.current) { videoRef.current.volume = next; videoRef.current.muted = next === 0; }
          setIsMuted(next === 0);
          break;
        }
        case "next-episode": onNextEpisode?.(); break;
        case "fullscreen": toggleFullscreen(); break;
        case "mute": toggleMute(); break;
        case "pip": togglePip(); break;
        case "rate": stepPlaybackRate(shortcut.direction); break;
        case "skip": runSkip(); break;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [playbackOffset, togglePlay, handleSeek, toggleFullscreen, toggleMute, shownSkip, runSkip, isMuted, volume, onNextEpisode, canPip, togglePip, stepPlaybackRate]);


  const matchingFiles = loadData && item && "episode_number" in item ? episodeCandidates(loadData.files,item as EpisodeSource) : [];
  const currentFile: FileInfo | undefined = loadData?.files[selectedFileIdx];
  const streamUrl = loadData && selectedFileIdx >= 0
    ? hlsMode ? `hls:${loadData.info_hash}:${selectedFileIdx}:${selectedAudioTrack}` : getStreamUrl(loadData.info_hash, selectedFileIdx, forceRemux, timeOffset, selectedAudioTrack,diagnostic)
    : "";

  // A new source resets playbackRate to defaultPlaybackRate, so set both.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.defaultPlaybackRate = playbackRate;
    video.playbackRate = playbackRate;
  }, [playbackRate, streamUrl, started]);

  // A new stream, codec or load state starts without the previous playback error.
  const playbackErrorKey = `${hlsMode}|${streamUrl}|${videoMeta?.video_codec}|${loading}|${needsFileSelection}`;
  const [prevPlaybackErrorKey, setPrevPlaybackErrorKey] = useState(playbackErrorKey);
  if (playbackErrorKey !== prevPlaybackErrorKey) {
    setPrevPlaybackErrorKey(playbackErrorKey);
    setPlaybackError(null);
  }

  useEffect(() => {
    // An unsupported HEVC track may still play its AAC audio without a media error.
    // Check both common HEVC profiles rather than waiting for onError alone.
    if (!hlsMode && isHevc(videoMeta?.video_codec) && videoRef.current &&
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
      metadata: (metadata, origin) => { receiveVideoMeta(metadata); setSubtitleOrigin(origin); },
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
    const key = episodeFileKey(seasonId, episodeNumber, loadData.info_hash, selectedFileIdx);
    if (previewRequested.current === key) return;
    previewRequested.current = key;
    void requestEpisodePreview(seasonId, episodeNumber, loadData.info_hash, selectedFileIdx, videoMeta.duration_sec, () => previewAlive.current).then(created => {
      if (!created || !previewAlive.current) return;
      setPreviewSaved(true);
      setTimeout(() => { if (previewAlive.current) setPreviewSaved(false); }, 4000);
    });
  }, [videoMeta, diagnostic?.season_id, episodeNumber, loadData, selectedFileIdx, episodes]);

  const pickSubtitleFile = async (file: File) => {
    if (!(await subtitles.pickFile(file))) return;
    setOptionsTab(null);
    triggerShowControls();
  };

  const handleSubtitleTrackSelect = (trackIdx: number | null) => {
    subtitles.selectTrack(trackIdx);
    setOptionsTab(null);
    triggerShowControls();
  };

  const handleVolumeChange = (val: number) => {
    setVolume(val);
    if (videoRef.current) {
      videoRef.current.volume = val;
      videoRef.current.muted = val === 0;
    }
    setIsMuted(val === 0);
    triggerShowControls();
  };

  // The file switcher below the player: a fresh start on the chosen file, defaults picked again.
  const selectFileManually = (index: number) => {
    resetSubtitleChoice();
    audioSelectionRef.current = false;
    resumePlaybackRef.current = true;
    setVideoMeta(null);
    setSelectedFileIdx(index);
    setTimeOffset(0);
    currentTimeRef.current = 0;
    setSelectedAudioTrack(0);
    setSelectedSubTrack(null);
    updateProgressDisplay(0);
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
    <PlayerTopBar
      item={item} animeTitle={animeTitle} episodeNumber={episodeNumber} pageMode={pageMode} showControls={showControls}
      previewSaved={previewSaved} stats={stats} onActivity={triggerShowControls} onClose={onClose} onChangeSource={onChangeSource}
    />
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
        {isFullscreen && containerEl ? createPortal(topBar, containerEl) : topBar}

        {/* Player & Content Area */}
        <div className="player-content flex-1 overflow-y-auto">
          {loading && !hlsMode ? (
            <PlayerStartup stage="connect" image={fallbackThumbnail} title={animeTitle} episode={episodeNumber} />
          ) : needsFileSelection && !hlsMode ? (
            <PlayerFileSelection
              matchingFiles={matchingFiles} allFiles={loadData?.files ?? []} episodeNumber={episodeNumber}
              failingOver={!!onPlaybackFailure} onSelect={(index) => { setSelectedFileIdx(index); setNeedsFileSelection(false); }}
            />
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
                ref={setContainer}
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
                        if(err.name==="NotAllowedError"){resumePlaybackRef.current=false;loadStartedAtRef.current=null;setIsPlaying(false);setNeedsPlaybackGesture(true);diagnosticEvent(diagnostic,"playback.gesture_required");return;}
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
                    setPlaybackError(mediaErrorMessage(mediaError?.code, videoMeta?.video_codec));
                  }}
                  onEnded={(event) => {
                    if (event.currentTarget !== videoRef.current) return;
                    if (onPlaybackFailure && endedEarly(playbackOffset + event.currentTarget.currentTime, totalDuration)) {
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
                  url={localSubtitle ? "" : subtitleUrl}
                  localContent={localSubtitle?.ass}
                  bitmap={subtitleBitmap}
                  timeOffset={playbackOffset}
                  style={subtitleStyle}
                  onError={subtitles.handleSubtitleError}
                />
                {debugMode && (
                  <PlayerDebugPanel
                    diagnostic={diagnostic} item={item} file={currentFile} fileIndex={selectedFileIdx} fileCount={loadData?.files.length}
                    meta={videoMeta} audioTrack={selectedAudioTrack} subtitleTrack={selectedSubTrack} remux={forceRemux}
                    timeOffset={playbackOffset} streamUrl={streamUrl} subtitleUrl={subtitleUrl} attempt={debugAttempt}
                  />
                )}
                {subtitleError && (
                  <div onMouseEnter={() => subtitles.setToastHovered(true)} onMouseLeave={() => subtitles.setToastHovered(false)} className="contents">
                    <ErrorAlert className="player-toast" message={subtitleError.message} code={subtitleError.code} reference={diagnostic?.playback_session_id} onClose={() => subtitles.setSubtitleError(null)} />
                  </div>
                )}
                </div>
                {needsPlaybackGesture&&<div className="absolute inset-0 z-20 flex items-end justify-center pb-44 pointer-events-none sm:items-center sm:pb-0"><button className="pointer-events-auto flex items-center gap-2 rounded-full bg-zinc-900/90 px-6 py-4 text-white border border-zinc-700" onClick={togglePlay}><Play size={22} />{t("Lecture")}</button></div>}
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
                    <PlayerTimeline
                      timeRef={timeDisplayRef} barRef={progressBarRef} bufferedRef={bufferedBarRef} playedRef={playedBarRef} knobRef={knobRef}
                      playbackOffset={playbackOffset} totalDuration={totalDuration} formattedDuration={videoMeta?.formatted_duration}
                      position={() => playbackOffset + currentTimeRef.current} onSeek={handleSeek}
                    />

                    <PlayerControls
                      isPlaying={isPlaying} onTogglePlay={togglePlay} onPrevEpisode={onPrevEpisode} onNextEpisode={onNextEpisode}
                      onSeekBy={(delta) => handleSeek(playbackOffset + currentTimeRef.current + delta)}
                      isMuted={isMuted} volume={volume} onToggleMute={toggleMute} onVolumeChange={handleVolumeChange}
                      onToggleEpisodes={episodes && episodes.length > 0 && onSelectEpisode ? () => setShowEpisodes(!showEpisodes) : undefined} showEpisodes={showEpisodes}
                      optionsOpen={optionsTab !== null} onToggleOptions={() => { setShowEpisodes(false); setOptionsTab(optionsTab ? null : "audio"); }}
                      partyActive={party.room !== null} partyCopied={partyCopied} onCopyPartyLink={copyPartyLink}
                      linkCopied={linkCopied} onCopyTimeLink={copyTimeLink}
                      onAirPlay={canAirPlay ? showAirPlay : undefined} onPip={canPip ? togglePip : undefined}
                      isFullscreen={isFullscreen} onToggleFullscreen={toggleFullscreen}
                    />
                  </div>
                </div>

                {nextCountdown !== null && (
                  <PlayerNextEpisodeCard seconds={nextCountdown} onStartNow={() => { setNextCountdown(null); onNextEpisode?.(); }} onCancel={() => setNextCountdown(null)} />
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
                    playbackRate={playbackRate}
                    rates={PLAYBACK_RATES}
                    onPlaybackRateChange={setPlaybackRate}
                    subtitleStyle={subtitleStyle}
                    onSubtitleStyleChange={updateSubtitleStyle}
                    localSubtitleName={localSubtitle?.name ?? null}
                    onPickSubtitleFile={(file) => void pickSubtitleFile(file)}
                    subtitleFileError={subtitles.subtitleFileError}
                  />
                )}
              </div>

              <PlayerDetails
                item={item} stats={stats} totalDuration={totalDuration} formattedDuration={videoMeta?.formatted_duration}
                fileSize={videoMeta?.total_bytes || currentFile?.length || item.size_bytes}
                files={!onPlaybackFailure ? loadData?.files : undefined} selectedFileIdx={selectedFileIdx} onSelectFile={selectFileManually}
                forceRemux={forceRemux} onForceRemuxChange={setForceRemux}
              />
            </div>
          )}
        </div>
      </div>
      {sourcePicker && typeof document !== "undefined" && createPortal(sourcePicker, document.fullscreenElement || document.body)}
    </div>
  );
};
