"use client";
import { diagnosticEvent, type PlaybackDiagnostic } from "@/lib/diagnostics";
import { useI18n } from "@/lib/i18n";
import { mediaTrackLabel, trackLanguageCode } from "@/lib/media-tracks";
import { copyText } from "@/lib/clipboard";

import { SubtitleRenderer } from "./SubtitleRenderer";
import { PlayerEpisodePicker } from "./PlayerEpisodePicker";
import { PlayerFailover, type FailoverInfo } from "./PlayerFailover";
import { PlayerOptionsModal, type AmbilightSettings, type PlayerOptionsTab } from "./PlayerOptionsModal";
import { PLAYBACK_TIMEOUTS } from "@/lib/playback-sources";
import { episodeFile, episodeCandidates } from "@/lib/episode-file";
import type { EpisodeInfo, EpisodeSource } from "@/types/api";
import { createPortal } from "react-dom";
import React, { useEffect, useState, useRef, useCallback } from "react";
import { TorrentItem, LoadTorrentResponse, SwarmStats, FileInfo, VideoMetadata } from "@/types/api";
import { loadTorrent, getTorrentStats, getStreamUrl, getSubtitleUrl, fetchVideoMetadata, formatBytes } from "@/lib/api";
import {
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
  SlidersHorizontal,
  List,
  Sun,
} from "lucide-react";

interface VideoPlayerModalProps {
  item: TorrentItem | null;
  diagnostic?: PlaybackDiagnostic;
  pageMode?: boolean;
  initialTime?: number;
  onProgress?: (position:number, duration:number)=>void;
  onPlaybackFailure?: (failure: { reason: string; position: number }) => void;
  onClose: () => void;
  animeTitle?: string;
  episodeNumber?: number;
  totalEpisodes?: number;
  onNextEpisode?: () => void;
  onPrevEpisode?: () => void;
  onChangeSource?: () => void;
  sourcePicker?: React.ReactNode;
  episodes?: EpisodeInfo[];
  onSelectEpisode?: (episode: number) => void;
  failover?: FailoverInfo;
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

export const VideoPlayerModal: React.FC<VideoPlayerModalProps> = ({
  item,
  onClose,
  animeTitle,
  episodeNumber,
  onNextEpisode,
  onPrevEpisode,
  onChangeSource,
  sourcePicker,
  episodes,
  onSelectEpisode,
  failover,
  pageMode = false,
  initialTime = 0,
  onPlaybackFailure,
  onProgress,
  diagnostic,
}) => {
  const { t, locale } = useI18n();
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
  const [errorCopied, setErrorCopied] = useState(false);
  const [toastHovered, setToastHovered] = useState(false);
  const handleSubtitleError = useCallback((message: string | null, code?: string) => {
    setSubtitleError(message ? { message, code } : null);
    if (message) diagnosticEvent(diagnostic, "playback.subtitle_failed", { error_code: code || "unknown" });
  }, [diagnostic]);
  const resumePlaybackRef = useRef(true);
  const failureReportedRef = useRef(false);
  const hasStartedRef = useRef(false);
  const lastProgressRef = useRef({ time: 0, at: 0 });
  const subtitleSelectionRef = useRef(false);
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

  if (itemHash !== prevItemHash) {
    setPrevItemHash(itemHash);
    setLoading(true);
    setError(null);
    setStats(null);
    setVideoMeta(null);
    setTimeOffset(0);
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

  // Direct DOM updates for smooth timeline
  const updateProgressDisplay = useCallback(
    (timeSec: number) => {
      const cur = timeOffset + timeSec;
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
    [timeOffset, totalDuration]
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
    const start = Math.min(100, Math.max(0, (timeOffset / totalDuration) * 100));
    const stop = Math.min(100, Math.max(start, ((timeOffset + end) / totalDuration) * 100));
    bar.style.left = `${start}%`;
    bar.style.width = `${stop - start}%`;
  }, [timeOffset, totalDuration]);

  useEffect(() => () => {
    if (hideControlsTimeoutRef.current) clearTimeout(hideControlsTimeoutRef.current);
  }, []);

  useEffect(()=>{
    if(videoMeta)diagnosticEvent(diagnostic,"playback.tracks",{video_codec:videoMeta.video_codec,audio_codec:videoMeta.audio_tracks?.find(t=>t.index===selectedAudioTrack)?.codec||'',audio_track:selectedAudioTrack,subtitle_track:selectedSubTrack??-1,duration:videoMeta.duration_sec});
  },[diagnostic,videoMeta,selectedAudioTrack,selectedSubTrack]);

  const reportFailure = useCallback((reason: string, error_code="playback_failed") => {
    if (!onPlaybackFailure || failureReportedRef.current) return;
    failureReportedRef.current = true;
    diagnosticEvent(diagnostic,"playback.failed",{reason,error_code,position:timeOffset+(videoRef.current?.currentTime||currentTimeRef.current)});
    onPlaybackFailure({ reason, position: timeOffset + (videoRef.current?.currentTime || currentTimeRef.current) });
  }, [onPlaybackFailure, timeOffset]);

  useEffect(() => {
    if (error) reportFailure(error,"torrent_metadata_failed");
    else if (needsFileSelection) reportFailure("L’épisode demandé n’est pas identifié sans ambiguïté dans ce pack.","episode_missing_or_ambiguous");
    else if (playbackError) reportFailure(playbackError,"media_error");
  }, [error, needsFileSelection, playbackError, reportFailure]);

  useEffect(() => {
    if (!onPlaybackFailure || !item) return;
    const deadline = setTimeout(() => reportFailure("Cette source ne fournit pas ses métadonnées à temps.","metadata_timeout"), PLAYBACK_TIMEOUTS.metadata);
    if (!loading) clearTimeout(deadline);
    return () => clearTimeout(deadline);
  }, [loading, item, onPlaybackFailure, reportFailure]);

  // Actual time advancement proves playback. Buffering can occur without an
  // error event, so a dead swarm must not hold this source indefinitely.
  useEffect(() => {
    if (!onPlaybackFailure || loading || needsFileSelection) return;
    const startedAt = Date.now();
    const poll = setInterval(() => {
      const video = videoRef.current;
      if (!video || !resumePlaybackRef.current) return;
      const now = Date.now();
      if (!hasStartedRef.current && now-startedAt >= PLAYBACK_TIMEOUTS.startup) {
        reportFailure("La lecture ne démarre pas sur cette source.","startup_timeout");
      } else if (hasStartedRef.current && now-lastProgressRef.current.at >= PLAYBACK_TIMEOUTS.stall) {
        reportFailure("Cette source ne fournit plus de vidéo.","swarm_stall");
      }
    }, 1000);
    return () => clearInterval(poll);
  }, [loading, needsFileSelection, onPlaybackFailure, reportFailure, timeOffset, selectedAudioTrack]);

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
  }, [triggerShowControls, isBuffering]);

  const handleSeek = useCallback((targetSec: number) => {
    diagnosticEvent(diagnostic,"playback.seek",{position:targetSec});
    let finalSec = targetSec;
    if (finalSec < 0) finalSec = 0;
    if (totalDuration > 0 && finalSec > totalDuration) finalSec = totalDuration;

    hasStartedRef.current = false;
    lastProgressRef.current = { time: 0, at: 0 };
    setIsBuffering(true);
    setForceRemux(true);
    setTimeOffset(finalSec);
    currentTimeRef.current = 0;
    setNeedsFileSelection(false);
    updateProgressDisplay(0);

    triggerShowControls();
  }, [totalDuration, updateProgressDisplay, triggerShowControls]);

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

    resumePlaybackRef.current = true;
    subtitleSelectionRef.current = false;
    let isMounted = true;
    const controller = new AbortController();
    setLoadData(null);
    setSelectedFileIdx(-1);
    setNeedsFileSelection(false);
    setFileSearch("");
    currentTimeRef.current = 0;

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
          if (data.main_video_metadata.subtitle_tracks && data.main_video_metadata.subtitle_tracks.length > 0) {
            subtitleSelectionRef.current = true;
            const frenchSub = data.main_video_metadata.subtitle_tracks.find((t) =>
              t.language.toLowerCase().includes("fre") ||
              t.language.toLowerCase().includes("fra") ||
              t.title.toLowerCase().includes("french") ||
              t.title.toLowerCase().includes("français") ||
              t.title.toLowerCase().includes("vostfr")
            );
            if (frenchSub) {
              setSelectedSubTrack(frenchSub.index);
            } else {
              const def = data.main_video_metadata.subtitle_tracks.find((t) => t.is_default);
              setSelectedSubTrack(def ? def.index : 0);
            }
          }
          if (data.main_video_metadata.audio_tracks && data.main_video_metadata.audio_tracks.length > 0) {
            const frenchAudio = data.main_video_metadata.audio_tracks.find((t) =>
              t.language.toLowerCase().includes("fre") ||
              t.language.toLowerCase().includes("fra") ||
              t.title.toLowerCase().includes("french") ||
              t.title.toLowerCase().includes("français") ||
              t.title.toLowerCase().includes("vf")
            );
            if (frenchAudio) {
              setSelectedAudioTrack(frenchAudio.index);
            }
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
      diagnosticEvent(diagnostic,"playback.abandoned",{position:timeOffset+currentTimeRef.current});
      controller.abort();
      if (pollIntervalRef.current) clearInterval(pollIntervalRef.current);
    };
  }, [item]);

  // 2. Poll Live Swarm Stats
  useEffect(() => {
    if (!loadData || !loadData.info_hash) return;

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
    if (!loadData || selectedFileIdx < 0) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const probe = async () => {
      try {
        const m = await fetchVideoMetadata(loadData.info_hash, selectedFileIdx, controller.signal,diagnostic);
        if (controller.signal.aborted) return;
        if (m && (m.duration_sec > 0 || m.video_codec)) {
          setVideoMeta(m);
          if (!subtitleSelectionRef.current && m.subtitle_tracks?.length) {
            const preferred = m.subtitle_tracks.find(t => /^(fre|fra|fr)$/i.test(t.language)) || m.subtitle_tracks.find(t => t.is_default) || m.subtitle_tracks[0];
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
  }, [loadData, selectedFileIdx]);

  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  // 4. Global Keyboard Shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('[role="dialog"]') || ["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes((e.target as HTMLElement).tagName)) {
        return;
      }

      const cur = timeOffset + currentTimeRef.current;

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
        case "KeyF":
          e.preventDefault();
          toggleFullscreen();
          break;
        case "KeyM":
          e.preventDefault();
          toggleMute();
          break;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [timeOffset, togglePlay, handleSeek, toggleFullscreen, toggleMute]);


  const matchingFiles = loadData && item && "episode_number" in item ? episodeCandidates(loadData.files,item as EpisodeSource) : [];
  const selectionFiles = (matchingFiles.length ? matchingFiles : loadData?.files.filter(file=>file.is_video) || []).filter(file=>file.path.toLowerCase().includes(fileSearch.toLowerCase()));
  const currentFile: FileInfo | undefined = loadData?.files[selectedFileIdx];
  const streamUrl = loadData && selectedFileIdx >= 0
    ? getStreamUrl(loadData.info_hash, selectedFileIdx, forceRemux, timeOffset, selectedAudioTrack,diagnostic)
    : "";

  useEffect(() => {
    setPlaybackError(null);
    // An unsupported HEVC track may still play its AAC audio without a media error.
    // Check both common HEVC profiles rather than waiting for onError alone.
    const codec = videoMeta?.video_codec?.toLowerCase();
    if ((codec === "hevc" || codec === "h265") && videoRef.current &&
        videoRef.current.readyState < HTMLMediaElement.HAVE_CURRENT_DATA &&
        !videoRef.current.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"') &&
        !videoRef.current.canPlayType('video/mp4; codecs="hvc1.2.4.L123.B0"')) {
      setPlaybackError("Ce navigateur ne prend pas en charge la vidéo H.265/HEVC. Choisissez une source H.264/AVC ou un navigateur compatible HEVC.");
    }
  }, [streamUrl, videoMeta?.video_codec, loading, needsFileSelection]);

  const subtitleUrl =
    loadData && selectedSubTrack !== null
      ? getSubtitleUrl(loadData.info_hash, selectedFileIdx, selectedSubTrack, "ass",diagnostic)
      : "";

  // Mount a fresh decoder for each source and restore the user's audio settings.
  useEffect(() => {
    const video = videoRef.current;
    if (video) video.volume = volume;
  }, [streamUrl, volume, loading, needsFileSelection]);

  const handleAudioTrackSelect = (trackIdx: number) => {
    setOptionsTab(null);
    if (trackIdx === selectedAudioTrack) return;
    const currentAbsoluteTime = timeOffset + (videoRef.current?.currentTime ?? currentTimeRef.current);
    hasStartedRef.current = false;
    lastProgressRef.current = { time: 0, at: 0 };
    setForceRemux(true);
    setSelectedAudioTrack(trackIdx);
    setIsBuffering(true);
    setTimeOffset(currentAbsoluteTime);
    currentTimeRef.current = 0;

    triggerShowControls();
  };

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
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(stage);
    observer.observe(dock);
    return () => observer.disconnect();
  }, [loading, error, needsFileSelection, item]);

  if (!item) return null;

  return (
    <div
      className={pageMode?"watch-player fixed inset-0 z-50 bg-black":"fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-sm p-2 sm:p-6 animate-in fade-in duration-150"}
      onClick={pageMode?undefined:onClose}
    >
      <div
        className={pageMode?"watch-player-shell":"relative flex flex-col w-full max-w-5xl max-h-[95vh] rounded-[28px] border border-zinc-800 bg-zinc-950 overflow-hidden shadow-2xl"}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Floating top bar */}
        <div className={`player-heading player-topbar${!showControls?" watch-heading-hidden":""}`}>
          <div className="flex min-w-0 items-center gap-2.5">
            <button
              aria-label={t(pageMode?"Voir les saisons":"Fermer le lecteur")}
              onClick={onClose}
              className="player-frost player-pill player-pill--icon"
            >
              <X className="h-5 w-5" />
            </button>
            <div className="player-frost player-pill min-w-0 !gap-2.5 !pl-2 !pr-5">
              {episodeNumber && <span className="player-chip player-chip--solid">EP {episodeNumber}</span>}
              <h2 className="truncate text-sm font-semibold" title={animeTitle || item.title}>
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

        {/* Player & Content Area */}
        <div className="player-content flex-1 overflow-y-auto">
          {loading ? (
            <div className="player-connecting" role="status" aria-live="polite">
              <svg className="watch-loading-ring" viewBox="0 0 72 72" aria-hidden="true">
                <circle className="watch-loading-track" cx="36" cy="36" r="32" />
                <circle className="watch-loading-arc" cx="36" cy="36" r="32" pathLength={100} />
              </svg>
              <h2 className="serif">{t("Connexion au swarm…")}</h2>
              <p>{t("Récupération des métadonnées et des premières pièces")}</p>
            </div>
          ) : needsFileSelection ? (
            <div className="p-6 pt-24 space-y-4">
              <p>{onPlaybackFailure ? t("Cet épisode ne peut pas être identifié dans ce pack. Essai de la source suivante…") : t("Choisissez le fichier correspondant à l’épisode {episode}. Aucun fichier n’a été lancé automatiquement.", {episode:episodeNumber ?? "—"})}</p>
              {!onPlaybackFailure && <>
              <input aria-label={t("Rechercher un fichier")} placeholder={t("Rechercher un fichier…")} value={fileSearch} onChange={event=>setFileSearch(event.target.value)} className="w-full bg-zinc-900 border border-zinc-700 p-3 rounded-full" />
              <p>{selectionFiles.length} {" "}{t("fichiers")}{matchingFiles.length ? t(" correspondant à cet épisode") : t(" vidéo")}</p>
              {selectionFiles.slice(0,50).map(file => <button key={file.index} className="block p-3 bg-zinc-900 rounded-2xl text-left w-full" onClick={() => { setSelectedFileIdx(file.index); setNeedsFileSelection(false); }}>{file.path}</button>)}
              {selectionFiles.length>50 && <p>{t("Affichage des 50 premiers fichiers. Affinez la recherche.")}</p>}
              </>}
            </div>
          ) : error ? (
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
                  key={streamUrl}
                  muted={isMuted}
                  ref={videoRef}
                  src={streamUrl}
                  playsInline
                  crossOrigin="anonymous"
                  onTimeUpdate={(event) => {
                  if(!hasStartedRef.current&&event.currentTarget===videoRef.current&&event.currentTarget.currentTime>lastProgressRef.current.time&&event.currentTarget.videoWidth>0)diagnosticEvent(diagnostic,"playback.started",{position:timeOffset+event.currentTarget.currentTime,width:event.currentTarget.videoWidth,height:event.currentTarget.videoHeight});
                    if (event.currentTarget === videoRef.current) {
                      const time = videoRef.current.currentTime;
                      if (time > lastProgressRef.current.time && videoRef.current.videoWidth > 0) {
                        hasStartedRef.current = true;
                        lastProgressRef.current = { time, at: Date.now() };
                      }
                      currentTimeRef.current = time;
                      onProgress?.(timeOffset + time, totalDuration);
                      updateProgressDisplay(videoRef.current.currentTime);
                      updateBuffered();
                    }
                  }}
                  onPlay={(event) => { if (event.currentTarget === videoRef.current) {resumePlaybackRef.current=true;setNeedsPlaybackGesture(false);setIsPlaying(true);} }}
                  onPause={(event) => { if (event.currentTarget === videoRef.current) setIsPlaying(false); }}
                  onWaiting={(event) => { if (event.currentTarget === videoRef.current) {setIsBuffering(true);diagnosticEvent(diagnostic,"playback.buffering",{position:timeOffset+event.currentTarget.currentTime,ready_state:event.currentTarget.readyState});} }}
                  onProgress={() => updateBuffered()}
                  onPlaying={(event) => { if (event.currentTarget === videoRef.current) { setIsBuffering(false); setStarted(true); } }}
                  onCanPlay={(event) => {
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
                    if (event.currentTarget !== videoRef.current) return;
                    const mediaError = event.currentTarget.error;
                    diagnosticEvent(diagnostic,"playback.media_error",{error_code:String(mediaError?.code||0),ready_state:event.currentTarget.readyState,network_state:event.currentTarget.networkState,video_codec:videoMeta?.video_codec||"",position:timeOffset+event.currentTarget.currentTime});
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
                    if (onPlaybackFailure && totalDuration > 0 && timeOffset + event.currentTarget.currentTime < totalDuration - 2) {
                      reportFailure("La lecture de cette source s’est interrompue avant la fin de l’épisode.","premature_end");
                      return;
                    }
                    if (onNextEpisode) {
                      onNextEpisode();
                    }
                  }}
                  onClick={togglePlay}
                  className="absolute inset-0 h-full w-full object-contain cursor-pointer"
                >
                  {t("Your browser does not support HTML5 video playback.")}</video>
                <SubtitleRenderer
                  key={`${streamUrl}-${subtitleUrl}`}
                  videoRef={videoRef}
                  url={subtitleUrl}
                  timeOffset={timeOffset}
                  onError={handleSubtitleError}
                />
                {subtitleError && (
                  <div role="alert" onMouseEnter={() => setToastHovered(true)} onMouseLeave={() => setToastHovered(false)} className="player-toast player-frost flex items-center gap-2 rounded-full py-1.5 pl-4 pr-1.5 text-xs text-red-300" style={{ background: "rgba(60,20,24,.5)" }}>
                    <span>{t(subtitleError.message)}</span>
                    {subtitleError.code && <span className="shrink-0 rounded-full bg-black/30 px-2 py-0.5 font-mono text-[10px] text-red-200" title={t("Code d’erreur")}>{subtitleError.code}{diagnostic?.playback_session_id ? ` · ${diagnostic.playback_session_id.slice(0, 8)}` : ""}</span>}
                    <button
                      type="button"
                      className="shrink-0 rounded-full bg-white/10 px-3 py-1 text-[11px] font-medium text-red-100 hover:bg-white/20"
                      onClick={async () => {
                        const report = [subtitleError.code && `Code: ${subtitleError.code}`, diagnostic?.playback_session_id && `Reference: ${diagnostic.playback_session_id}`, t(subtitleError.message)].filter(Boolean).join("\n");
                        if (await copyText(report)) { setErrorCopied(true); setTimeout(() => setErrorCopied(false), 2000); }
                      }}
                    >{t(errorCopied ? "Copié" : "Copier")}</button>
                    <button type="button" aria-label={t("Fermer")} onClick={() => setSubtitleError(null)} className="grid h-7 w-7 shrink-0 place-items-center rounded-full hover:bg-white/10"><X className="h-3.5 w-3.5" /></button>
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

                {/* Buffering Indicator */}
                {isBuffering && (
                  <div className="player-frost absolute right-6 top-24 z-20 flex items-center gap-2 rounded-full px-3.5 py-1.5 font-mono text-xs text-zinc-200">
                    <Loader2 className="h-3 w-3 animate-spin text-zinc-400" />
                    <span>{t("Buffering stream...")}</span>
                  </div>
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
                      currentEpisode={episodeNumber}
                      onClose={() => setShowEpisodes(false)}
                      onSelect={(number) => { setShowEpisodes(false); if (number !== episodeNumber) onSelectEpisode(number); }}
                    />
                  )}
                  <div className="player-frost flex flex-col gap-3 px-4 py-3.5 text-zinc-200 sm:gap-3.5 sm:px-[18px] sm:py-4" style={{ borderRadius: "var(--radius-dock, 32px)" }}>
                    {/* Scrubber */}
                    <div className="flex items-center gap-3.5 px-1.5">
                      <span ref={timeDisplayRef} className="min-w-11 font-mono text-xs text-zinc-50">{formatTime(timeOffset)}</span>
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
                    <div className="flex items-center justify-between gap-3 text-xs">
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
                        <button onClick={() => handleSeek(timeOffset + currentTimeRef.current - 10)} className="player-pill player-pill--icon" aria-label={t("Rewind 10s (←)")} title={t("Rewind 10s (←)")}>
                          <RotateCcw className="h-[18px] w-[18px]" />
                        </button>
                        <button onClick={() => handleSeek(timeOffset + currentTimeRef.current + 10)} className="player-pill player-pill--icon" aria-label={t("Forward 10s (→)")} title={t("Forward 10s (→)")}>
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
                            className="h-1.5 w-20 cursor-pointer appearance-none rounded-full bg-white/20 accent-white"
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
                          <SlidersHorizontal className="h-4 w-4" /><span className="player-label">{t("Réglages")}</span>
                        </button>
                        <button onClick={toggleFullscreen} className="player-pill player-pill--icon" aria-label={t("Toggle Fullscreen (F)")} title={t("Toggle Fullscreen (F)")}>
                          {isFullscreen ? <Minimize className="h-[18px] w-[18px]" /> : <Maximize className="h-[18px] w-[18px]" />}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>

                {failover && !started && <PlayerFailover info={failover} onChangeSource={onChangeSource} />}

                {optionsTab && (
                  <PlayerOptionsModal
                    tab={optionsTab}
                    onTabChange={setOptionsTab}
                    onClose={() => setOptionsTab(null)}
                    audioOptions={(videoMeta?.audio_tracks || []).map((track) => ({ index: track.index, title: track.title, label: mediaTrackLabel(track, videoMeta?.audio_tracks || [], locale) }))}
                    selectedAudio={selectedAudioTrack}
                    audioHint={videoMeta?.audio_tracks?.length && !videoMeta.audio_tracks.some(track => trackLanguageCode(track.language) === "fr") ? t(videoMeta.audio_tracks.some(track => !track.language || track.language === "und") ? "VF non confirmée pour ce fichier." : "VF indisponible dans ce fichier.") : null}
                    onSelectAudio={handleAudioTrackSelect}
                    subtitleOptions={(videoMeta?.subtitle_tracks || []).map((track) => ({ index: track.index, title: track.title, label: mediaTrackLabel(track, videoMeta?.subtitle_tracks || [], locale) }))}
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
                        resumePlaybackRef.current = true;
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
