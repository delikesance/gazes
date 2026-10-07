"use client";
import { useEffect, useRef, useState } from "react";
import type { LoadTorrentResponse, SwarmStats, TorrentItem, VideoMetadata } from "@/types/api";
import { getTorrentStats, loadTorrent } from "./api";
import { diagnosticEvent, type PlaybackDiagnostic } from "./diagnostics";
import { initialFileIndex, isLibrarySource, libraryLoad, matchedFileIndex } from "./player-state";

/** The source's torrent metadata, chosen file and swarm stats; reset on render when the item changes. */
export function useTorrentState(item: TorrentItem | null) {
  const [loading, setLoading] = useState(() => !libraryLoad(item));
  const [error, setError] = useState<string | null>(null);
  const [loadData, setLoadData] = useState<LoadTorrentResponse | null>(() => libraryLoad(item));
  const [needsFileSelection, setNeedsFileSelection] = useState(false);
  const [selectedFileIdx, setSelectedFileIdx] = useState<number>(() => initialFileIndex(item));
  const [stats, setStats] = useState<SwarmStats | null>(null);
  const pollIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const itemHash = item?.info_hash || item?.id || null;
  const [prevItemHash, setPrevItemHash] = useState<string | null>(itemHash);
  if (itemHash !== prevItemHash) {
    setPrevItemHash(itemHash);
    setLoading(true);
    setError(null);
    setStats(null);
    if (item) {
      setLoadData(libraryLoad(item));
      setSelectedFileIdx(initialFileIndex(item));
      setNeedsFileSelection(false);
      setLoading(!libraryLoad(item));
    }
  }

  return {
    loading, error, loadData, needsFileSelection, setNeedsFileSelection, selectedFileIdx, setSelectedFileIdx, stats,
    setLoading, setError, setLoadData, setStats, pollIntervalRef,
  };
}

interface TorrentLoadInput {
  item: TorrentItem | null;
  diagnostic?: PlaybackDiagnostic;
  /** Runs first on each new item: per-source refs go back to their start values. */
  onLoadStart: () => void;
  /** Read when the item is left, for the abandon diagnostic. */
  abandonedPosition: () => number;
  /** The main video's probe, when the torrent load already carries it for the chosen file. */
  onMainMetadata: (meta: VideoMetadata) => void;
}

/** Loads torrent metadata for each new item, then polls swarm stats every 5 s (torrents only). */
export function useTorrentLoad(torrent: ReturnType<typeof useTorrentState>, { item, diagnostic, onLoadStart, abandonedPosition, onMainMetadata }: TorrentLoadInput) {
  const { loadData, setLoadData, setNeedsFileSelection, setSelectedFileIdx, setLoading, setError, setStats, pollIntervalRef } = torrent;

  // 1. Load Torrent Metadata
  useEffect(() => {
    if (!item) return;

    onLoadStart();
    let isMounted = true;
    const controller = new AbortController();

    // Library copies are plain files on the server: their load data is set on render, nothing to fetch.
    if (libraryLoad(item)) {
      return () => {
        isMounted = false;
        diagnosticEvent(diagnostic,"playback.abandoned",{position:abandonedPosition()});
        controller.abort();
      };
    }

    loadTorrent(item.magnet_uri, {metadataOnly: true, signal:controller.signal,diagnostic})
      .then((data) => {
        if (!isMounted) return;
        setLoadData(data);
        const matched = matchedFileIndex(data, item);
        const initialIdx = matched === null ? -1 : matched;
        setNeedsFileSelection(matched === null);
        const selected=data.files.find(f=>f.index===initialIdx);
        diagnosticEvent(diagnostic,selected?'playback.file_selected':'playback.file_rejected',{file_index:initialIdx,file_path:selected?.path||'',file_count:data.files.length,reason:selected?'episode_match':'episode_missing_or_ambiguous'});
        setSelectedFileIdx(initialIdx);
        if (data.main_video_metadata && initialIdx === data.main_video_index) {
          onMainMetadata(data.main_video_metadata);

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
      diagnosticEvent(diagnostic,"playback.abandoned",{position:abandonedPosition()});
      controller.abort();
      if (pollIntervalRef.current) clearInterval(pollIntervalRef.current);
    };
  }, [item]);

  // 2. Poll Live Swarm Stats
  useEffect(() => {
    if (!loadData || !loadData.info_hash || isLibrarySource(item)) return;

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
}
