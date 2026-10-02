"use client";
import { diagnosticEvent, type PlaybackDiagnostic } from "@/lib/diagnostics";
import { useI18n } from "@/lib/i18n";

import { SubtitleRenderer } from "./SubtitleRenderer";
import { PLAYBACK_TIMEOUTS } from "@/lib/playback-sources";
import { episodeFile, episodeCandidates, episodeQualities } from "@/lib/episode-file";
import type { EpisodeSource } from "@/types/api";
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
  Headphones,
  MessageSquare,
  SkipForward,
  SkipBack,
  Layers,
  Loader2,
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
  pageMode = false,
  initialTime = 0,
  onPlaybackFailure,
  onProgress,
  diagnostic,
}) => {
  const { t } = useI18n();
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
  const [showAudioMenu, setShowAudioMenu] = useState<boolean>(false);
  const [showSubMenu, setShowSubMenu] = useState<boolean>(false);
  const [showQualityMenu,setShowQualityMenu]=useState(false);

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
  const videoRef = useRef<HTMLVideoElement>(null);
  const [subtitleError, setSubtitleError] = useState<string | null>(null);
  const resumePlaybackRef = useRef(true);
  const failureReportedRef = useRef(false);
  const hasStartedRef = useRef(false);
  const lastProgressRef = useRef({ time: 0, at: 0 });
  const subtitleSelectionRef = useRef(false);
  const progressBarRef = useRef<HTMLDivElement>(null);
  const playedBarRef = useRef<HTMLDivElement>(null);
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
        setShowAudioMenu(false);
        setShowSubMenu(false);
        setShowQualityMenu(false);
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
      if (["INPUT", "TEXTAREA", "SELECT"].includes((e.target as HTMLElement).tagName)) {
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
  const qualities=loadData&&item&&"episode_number" in item?episodeQualities(loadData.files,item as EpisodeSource):[];
  const changeQuality=(index:number)=>{
   setShowQualityMenu(false);if(index===selectedFileIdx)return;
   const position=timeOffset+(videoRef.current?.currentTime||0);
   hasStartedRef.current=false;lastProgressRef.current={time:0,at:0};
   setSelectedFileIdx(index);setTimeOffset(position);setForceRemux(position>0);
   setSelectedAudioTrack(0);setSelectedSubTrack(null);subtitleSelectionRef.current=false;
   setVideoMeta(null);setPlaybackError(null);setIsBuffering(true);currentTimeRef.current=0;
  };
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
    setShowAudioMenu(false);
    if (trackIdx === selectedAudioTrack) return;
    const currentAbsoluteTime = timeOffset + (videoRef.current?.currentTime ?? currentTimeRef.current);
    hasStartedRef.current = false;
    lastProgressRef.current = { time: 0, at: 0 };
    setForceRemux(true);
    setSelectedAudioTrack(trackIdx);
    setShowAudioMenu(false);
    setIsBuffering(true);
    setTimeOffset(currentAbsoluteTime);
    currentTimeRef.current = 0;

    triggerShowControls();
  };

  const handleSubtitleTrackSelect = (trackIdx: number | null) => {
    subtitleSelectionRef.current = true;
    setSelectedSubTrack(trackIdx);
    setShowSubMenu(false);
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
    navigator.clipboard.writeText(item.magnet_uri);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (!item) return null;

  return (
    <div
      className={pageMode?"watch-player fixed inset-0 z-50 bg-black":"fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-sm p-2 sm:p-6 animate-in fade-in duration-150"}
      onClick={pageMode?undefined:onClose}
    >
      <div
        className={pageMode?"watch-player-shell":"relative flex flex-col w-full max-w-5xl max-h-[95vh] rounded-2xl border border-zinc-800 bg-zinc-950 overflow-hidden shadow-2xl"}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className={`player-heading flex items-center justify-between border-b border-zinc-800 px-4 sm:px-5 py-3 bg-zinc-900/40${pageMode&&!showControls?" watch-heading-hidden":""}`}>
          <div className="flex items-center gap-2.5 overflow-hidden">

            <div className="truncate">
              <div className="flex items-center gap-2">
                {episodeNumber && (
                  <span className="rounded bg-zinc-800 border border-zinc-700 px-1.5 py-0.5 text-[10px] font-mono text-zinc-200 uppercase">
                    EP {episodeNumber}
                  </span>
                )}
                <h2 className="truncate text-xs sm:text-sm font-medium text-zinc-100" title={animeTitle || item.title}>
                  {animeTitle || item.anime_details?.display_title || item.title}
                </h2>
              </div>

            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            {onChangeSource && (
              <button
                onClick={onChangeSource}
                title={t("Change source for this episode")}
                className="flex items-center gap-1 rounded-lg bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 px-2.5 py-1 text-xs font-medium text-zinc-300 hover:text-white transition-colors cursor-pointer"
              >
                <Layers className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t("Sources")}</span>
              </button>
            )}

            <button
              aria-label={t(pageMode?"Voir les saisons":"Fermer le lecteur")}
              onClick={onClose}
              className="rounded-lg p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-white transition-colors cursor-pointer"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* Player & Content Area */}
        <div className="player-content flex-1 overflow-y-auto">
          {loading ? (
            <div className="flex flex-col items-center justify-center py-28 space-y-3">
              <Loader2 className="h-8 w-8 text-zinc-400 animate-spin" />
              <div className="text-center">
                <p className="text-xs text-zinc-300">{t("Connecting to BitTorrent swarm...")}</p>
                <p className="text-[11px] font-mono text-zinc-500 mt-0.5">{t("Fetching metadata & sequential pieces")}</p>
              </div>
            </div>
          ) : needsFileSelection ? (
            <div className="p-6 space-y-4">
              <p>{onPlaybackFailure ? t("Cet épisode ne peut pas être identifié dans ce pack. Essai de la source suivante…") : t("Choisissez le fichier correspondant à l’épisode {episode}. Aucun fichier n’a été lancé automatiquement.", {episode:episodeNumber ?? "—"})}</p>
              {!onPlaybackFailure && <>
              <input aria-label={t("Rechercher un fichier")} placeholder={t("Rechercher un fichier…")} value={fileSearch} onChange={event=>setFileSearch(event.target.value)} className="w-full bg-zinc-900 border border-zinc-700 p-3 rounded-lg" />
              <p>{selectionFiles.length} {" "}{t("fichiers")}{matchingFiles.length ? t(" correspondant à cet épisode") : t(" vidéo")}</p>
              {selectionFiles.slice(0,50).map(file => <button key={file.index} className="block p-3 bg-zinc-900 rounded-lg text-left w-full" onClick={() => { setSelectedFileIdx(file.index); setNeedsFileSelection(false); }}>{file.path}</button>)}
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
                className={`player-video-box relative w-full bg-black select-none group overflow-hidden flex flex-col${pageMode?"":" aspect-video"}`}
              >
                <div className="relative flex-1 min-h-0 overflow-hidden" data-player-stage>
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
                    }
                  }}
                  onPlay={(event) => { if (event.currentTarget === videoRef.current) {resumePlaybackRef.current=true;setNeedsPlaybackGesture(false);setIsPlaying(true);} }}
                  onPause={(event) => { if (event.currentTarget === videoRef.current) setIsPlaying(false); }}
                  onWaiting={(event) => { if (event.currentTarget === videoRef.current) {setIsBuffering(true);diagnosticEvent(diagnostic,"playback.buffering",{position:timeOffset+event.currentTarget.currentTime,ready_state:event.currentTarget.readyState});} }}
                  onPlaying={(event) => { if (event.currentTarget === videoRef.current) setIsBuffering(false); }}
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
                  onError={setSubtitleError}
                />
                {subtitleError && <p role="alert" className="absolute top-3 left-3 z-20 rounded bg-black/80 p-2 text-xs text-red-300">{t(subtitleError)}</p>}
                </div>
                {needsPlaybackGesture&&<div className="absolute inset-0 z-20 flex items-center justify-center pointer-events-none"><button className="pointer-events-auto flex items-center gap-2 rounded-full bg-zinc-900/90 px-6 py-4 text-white border border-zinc-700" onClick={togglePlay}><Play size={22} />{t("Lecture")}</button></div>}
                {playbackError && (
                  <div role="alert" className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-4 bg-black/90 p-6 text-center">
                    <p className="text-sm text-zinc-200">{t(playbackError)}</p>
                    <button className="rounded-lg bg-white px-4 py-2 text-black" onClick={() => { setPlaybackError(null); videoRef.current?.load(); }}>{t("Réessayer")}</button>
                    {onChangeSource && <button className="underline" onClick={onChangeSource}>{t("Changer de source")}</button>}
                  </div>
                )}

                {/* Buffering Indicator */}
                {isBuffering && (
                  <div className="absolute top-4 right-4 flex items-center gap-2 rounded-lg bg-zinc-950/90 px-3 py-1 text-xs text-zinc-300 border border-zinc-800 z-20 font-mono">
                    <Loader2 className="h-3 w-3 animate-spin text-zinc-400" />
                    <span>{t("Buffering stream...")}</span>
                  </div>
                )}

                {/* Minimalist Overlay Control Bar */}
                <div
                  data-player-controls
                  className={`relative shrink-0 pointer-events-auto bg-zinc-950/90 border-t border-zinc-800/80 p-3 sm:p-4 z-10 ${
                    showControls ? "visible" : "invisible !pointer-events-none"
                  }`}
                >
                  {/* Scrubber */}
                  <div
                    ref={progressBarRef}
                    onClick={handleProgressBarClick}
                    onMouseMove={handleProgressBarMouseMove}
                    onMouseLeave={() => setHoverTime(null)}
                    className="relative mb-3 h-1.5 w-full cursor-pointer rounded-full bg-zinc-800 hover:h-2 transition-all group/bar"
                  >
                    <div
                      ref={playedBarRef}
                      className="absolute top-0 left-0 h-full rounded-full bg-white"
                      style={{ width: "0%" }}
                    />
                    <div
                      ref={knobRef}
                      className="absolute top-1/2 -mt-1.5 -ml-1.5 h-3 w-3 rounded-full bg-white opacity-0 group-hover/bar:opacity-100 transition-opacity"
                      style={{ left: "0%" }}
                    />

                    {hoverTime !== null && (
                      <div
                        className="absolute bottom-3 -translate-x-1/2 rounded bg-zinc-900 border border-zinc-700 px-2 py-0.5 text-[10px] font-mono text-white shadow-lg pointer-events-none"
                        style={{ left: `${hoverPosition}%` }}
                      >
                        {formatTime(hoverTime)}
                      </div>
                    )}
                  </div>

                  {/* Controls Row */}
                  <div className="flex items-center justify-between text-zinc-200 text-xs">
                    {/* Left: Play/Pause, Navigation, Seek, Volume, Timecode */}
                    <div className="flex items-center gap-2">
                      {onPrevEpisode && (
                        <button
                          onClick={onPrevEpisode}
                          className="rounded p-1 hover:bg-zinc-800 text-zinc-400 hover:text-white transition-colors cursor-pointer"
                          title={t("Previous Episode")}
                        >
                          <SkipBack className="h-4 w-4" />
                        </button>
                      )}

                      <button
                        onClick={togglePlay}
                        className="rounded p-1 text-white hover:text-zinc-300 transition-colors cursor-pointer"
                        title={isPlaying ? t("Pause (Space)") : t("Play (Space)")}
                      >
                        {isPlaying ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4 fill-current" />}
                      </button>

                      {onNextEpisode && (
                        <button
                          onClick={onNextEpisode}
                          className="rounded p-1 hover:bg-zinc-800 text-zinc-400 hover:text-white transition-colors cursor-pointer"
                          title={t("Next Episode")}
                        >
                          <SkipForward className="h-4 w-4" />
                        </button>
                      )}

                      <button
                        onClick={() => handleSeek(timeOffset + currentTimeRef.current - 10)}
                        className="rounded p-1 text-zinc-400 hover:text-white transition-colors cursor-pointer"
                        title={t("Rewind 10s (←)")}
                      >
                        <RotateCcw className="h-3.5 w-3.5" />
                      </button>

                      <button
                        onClick={() => handleSeek(timeOffset + currentTimeRef.current + 10)}
                        className="rounded p-1 text-zinc-400 hover:text-white transition-colors cursor-pointer"
                        title={t("Forward 10s (→)")}
                      >
                        <RotateCw className="h-3.5 w-3.5" />
                      </button>

                      {/* Volume */}
                      <div className="flex items-center gap-1 ml-1">
                        <button
                          onClick={toggleMute}
                          className="rounded p-1 text-zinc-400 hover:text-white transition-colors cursor-pointer"
                          title={t(isMuted ? "Unmute (M)" : "Mute (M)")}
                        >
                          {isMuted || volume === 0 ? (
                            <VolumeX className="h-3.5 w-3.5 text-zinc-400" />
                          ) : volume < 0.5 ? (
                            <Volume1 className="h-3.5 w-3.5" />
                          ) : (
                            <Volume2 className="h-3.5 w-3.5" />
                          )}
                        </button>
                        <input
                          aria-label={t("Volume")}
                          type="range"
                          min="0"
                          max="1"
                          step="0.05"
                          value={isMuted ? 0 : volume}
                          onChange={handleVolumeChange}
                          className="w-14 h-1 bg-zinc-700 rounded appearance-none cursor-pointer accent-white"
                        />
                      </div>

                      {/* Timecode */}
                      <div className="font-mono text-[11px] text-zinc-400 select-none pl-2">
                        <span ref={timeDisplayRef} className="text-zinc-100 font-medium">
                          {formatTime(timeOffset)}
                        </span>
                        <span className="text-zinc-600 mx-1">/</span>
                        <span>
                          {totalDuration > 0 ? formatTime(totalDuration) : videoMeta?.formatted_duration || "--:--"}
                        </span>
                      </div>
                    </div>

                    {/* Right: Audio, Subtitles, Quality, Fullscreen */}
                    <div className="flex items-center gap-1.5">
                      {/* Audio */}
                      <div className="relative">
                        <button
                          onClick={() => {
                            setShowAudioMenu(!showAudioMenu);
                            setShowSubMenu(false);
                          }}
                          className={`flex items-center gap-1 rounded px-2 py-1 transition-colors cursor-pointer ${
                            showAudioMenu ? "bg-zinc-800 text-white" : "hover:bg-zinc-800 text-zinc-400"
                          }`}
                          title={t("Select Audio Track")}
                        >
                          <Headphones className="h-3.5 w-3.5" />
                          <span className="hidden sm:inline text-[11px]">{t("Audio")}</span>
                        </button>

                        {showAudioMenu && (
                          <div className="absolute bottom-8 right-0 w-52 rounded-xl border border-zinc-800 bg-zinc-900 p-1.5 shadow-2xl z-30 animate-in fade-in duration-100">
                            <p className="px-2.5 py-1 text-[10px] font-mono text-zinc-400 border-b border-zinc-800 mb-1">
                              {t("Audio Tracks")}</p>
                            {videoMeta?.audio_tracks && videoMeta.audio_tracks.length > 0 ? (
                              videoMeta.audio_tracks.map((track) => (
                                <button
                                  key={track.index}
                                  onClick={() => handleAudioTrackSelect(track.index)}
                                  className={`flex w-full items-center justify-between rounded px-2 py-1.5 text-xs text-left transition-colors cursor-pointer ${
                                    selectedAudioTrack === track.index
                                      ? "bg-zinc-800 text-white font-medium"
                                      : "hover:bg-zinc-800/60 text-zinc-300"
                                  }`}
                                >
                                  <span className="truncate">{track.title}</span>
                                  {selectedAudioTrack === track.index && <Check className="h-3.5 w-3.5 ml-1 shrink-0" />}
                                </button>
                              ))
                            ) : (
                              <div className="px-2.5 py-2 text-xs text-zinc-400">{t("Default Audio")}</div>
                            )}
                          </div>
                        )}
                      </div>

                      {/* Subtitles (CC) */}
                      <div className="relative">
                        <button
                          onClick={() => {
                            setShowSubMenu(!showSubMenu);
                            setShowAudioMenu(false);
                          }}
                          className={`flex items-center gap-1 rounded px-2 py-1 transition-colors cursor-pointer ${
                            selectedSubTrack !== null
                              ? "bg-zinc-800 text-white"
                              : showSubMenu
                              ? "bg-zinc-800 text-white"
                              : "hover:bg-zinc-800 text-zinc-400"
                          }`}
                          title={t("Select Subtitles")}
                        >
                          <MessageSquare className="h-3.5 w-3.5" />
                          <span className="hidden sm:inline text-[11px]">CC</span>
                        </button>

                        {showSubMenu && (
                          <div className="absolute bottom-8 right-0 w-56 rounded-xl border border-zinc-800 bg-zinc-900 p-1.5 shadow-2xl z-30 max-h-60 overflow-y-auto animate-in fade-in duration-100">
                            <p className="px-2.5 py-1 text-[10px] font-mono text-zinc-400 border-b border-zinc-800 mb-1">
                              {t("Subtitles")}</p>
                            <button
                              onClick={() => handleSubtitleTrackSelect(null)}
                              className={`flex w-full items-center justify-between rounded px-2 py-1.5 text-xs text-left transition-colors cursor-pointer ${
                                selectedSubTrack === null
                                  ? "bg-zinc-800 text-white font-medium"
                                  : "hover:bg-zinc-800/60 text-zinc-300"
                              }`}
                            >
                              <span>{t("Off")}</span>
                              {selectedSubTrack === null && <Check className="h-3.5 w-3.5 ml-1 shrink-0" />}
                            </button>

                            {videoMeta?.subtitle_tracks && videoMeta.subtitle_tracks.length > 0 ? (
                              videoMeta.subtitle_tracks.map((track) => (
                                <button
                                  key={track.index}
                                  onClick={() => handleSubtitleTrackSelect(track.index)}
                                  className={`flex w-full items-center justify-between rounded px-2 py-1.5 text-xs text-left transition-colors cursor-pointer ${
                                    selectedSubTrack === track.index
                                      ? "bg-zinc-800 text-white font-medium"
                                      : "hover:bg-zinc-800/60 text-zinc-300"
                                  }`}
                                >
                                  <span className="truncate">{track.title}</span>
                                  {selectedSubTrack === track.index && <Check className="h-3.5 w-3.5 ml-1 shrink-0" />}
                                </button>
                              ))
                            ) : null}
                          </div>
                        )}
                      </div>

                      <div className="relative">
                       <button disabled={qualities.length<2} aria-label={t("Qualité")} aria-expanded={showQualityMenu} title={t(qualities.length<2?"Une seule qualité disponible":"Choisir la qualité")} onClick={()=>{setShowQualityMenu(!showQualityMenu);setShowAudioMenu(false);setShowSubMenu(false);}} className="rounded bg-zinc-800 px-2 py-1 text-[10px] font-mono text-zinc-200 disabled:!opacity-100">{videoMeta?.resolution||qualities.find(q=>q.file.index===selectedFileIdx)?.height&&`${qualities.find(q=>q.file.index===selectedFileIdx)?.height}p`||t("Original")}</button>
                       {showQualityMenu&&<div className="absolute bottom-8 right-0 w-32 rounded-xl border border-zinc-800 bg-zinc-900 p-1.5 z-30" role="group" aria-label={t("Qualité")}>{qualities.map(q=><button key={q.height} className="flex w-full justify-between rounded px-3 py-2 text-xs text-white hover:bg-zinc-800" onClick={()=>changeQuality(q.file.index)} aria-pressed={q.file.index===selectedFileIdx}>{q.height}p{q.file.index===selectedFileIdx&&<Check size={14}/>}</button>)}</div>}
                      </div>
                      <button
                        onClick={toggleFullscreen}
                        className="rounded p-1 hover:bg-zinc-800 text-zinc-400 hover:text-white transition-colors cursor-pointer"
                        title={t("Toggle Fullscreen (F)")}
                      >
                        {isFullscreen ? <Minimize className="h-3.5 w-3.5" /> : <Maximize className="h-3.5 w-3.5" />}
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              {/* Swarm & Metadata Telemetry HUD */}
              <div className="player-telemetry border-b border-zinc-800 bg-zinc-900/20 p-4">
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                  {/* Seeders */}
                  <div className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("Seeders")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {stats?.active_seeders ?? item.seeders} <span className="text-zinc-500 font-normal">({stats?.total_peers ?? 0} {" "}{t("peers)")}</span>
                    </p>
                  </div>

                  {/* Download Speed */}
                  <div className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("Download Speed")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {stats ? `${formatBytes(stats.download_rate_bps)}/s` : t("Buffering...")}
                    </p>
                  </div>

                  {/* Duration */}
                  <div className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-2.5">
                    <p className="text-[11px] text-zinc-400">{t("Duration")}</p>
                    <p className="font-mono text-zinc-200 font-medium mt-0.5">
                      {totalDuration > 0 ? formatTime(totalDuration) : videoMeta?.formatted_duration || t("Detecting...")}
                    </p>
                  </div>

                  {/* File Size */}
                  <div className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-2.5">
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
                      className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-200 focus:border-zinc-500 focus:outline-none font-mono"
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
                    className="flex items-center gap-1.5 rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 hover:text-white transition-colors cursor-pointer"
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
    </div>
  );
};
