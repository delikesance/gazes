"use client";

import { useState, useSyncExternalStore } from "react";
import { Bug, X } from "lucide-react";
import { copyText } from "@/lib/clipboard";
import type { PlaybackDiagnostic } from "@/lib/diagnostics";
import type { EpisodeSource, FileInfo, TorrentItem, VideoMetadata } from "@/types/api";

export interface DebugAttempt { index: number; count: number; tried: string[] }

const subscribe = () => () => {};
const readFlag = () => new URLSearchParams(window.location.search).get("debug") === "true";
/** True when the page URL carries `?debug=true`. Server snapshot is false, so no hydration mismatch. */
export function useDebugMode(): boolean {
  return useSyncExternalStore(subscribe, readFlag, () => false);
}

type Row = [label: string, value: string | number | undefined | null];

function absolute(url: string): string {
  try { return new URL(url, window.location.origin).toString(); } catch { return url; }
}

/** `?debug=true` overlay: tracing id plus the torrent, file, tracks and URLs actually in use. */
export function PlayerDebugPanel({ diagnostic, item, file, fileIndex, fileCount, meta, audioTrack, subtitleTrack, remux, timeOffset, streamUrl, subtitleUrl, attempt }: {
  diagnostic?: PlaybackDiagnostic;
  item: TorrentItem | EpisodeSource | null;
  file?: FileInfo;
  fileIndex: number;
  fileCount?: number;
  meta: VideoMetadata | null;
  audioTrack: number;
  subtitleTrack: number | null;
  remux: boolean;
  timeOffset: number;
  streamUrl: string;
  subtitleUrl: string;
  attempt?: DebugAttempt;
}) {
  const [open, setOpen] = useState(true);
  const [copied, setCopied] = useState(false);
  const source = item && "episode_number" in item ? item : null;
  const audio = meta?.audio_tracks?.find((track) => track.index === audioTrack);
  const subtitle = meta?.subtitle_tracks?.find((track) => track.index === subtitleTrack);
  const trace = diagnostic?.playback_session_id;

  const sections: [string, Row[]][] = [
    ["Tracing", [
      ["trace id", trace], ["attempt id", diagnostic?.attempt_id],
      ["anime / season / episode", [diagnostic?.anime_id, diagnostic?.season_id, diagnostic?.episode].join(" / ")],
      ["attempt", attempt ? `${attempt.index + 1} / ${attempt.count}` : undefined],
      ["already tried", attempt?.tried.length ? attempt.tried.join(", ") : undefined],
    ]],
    ["Torrent", [
      ["title", item?.title], ["infohash", item?.info_hash], ["provider", source?.provider],
      ["release group", source?.release_group], ["quality", source?.quality], ["language", source?.language_label],
      ["seeders / leechers", item ? `${item.seeders} / ${item.leechers}` : undefined], ["size", item?.size_display],
      ["batch", source ? String(source.is_batch) : undefined],
      ["score", source ? `${source.score_rank} (${Object.entries(source.score_breakdown || {}).map(([k, v]) => `${k} ${v}`).join(", ")})` : undefined],
      ["numbering", source ? `season ${source.season_number} · ep ${source.episode_number} · tagged ${source.tagged_episode ?? "-"} · absolute ${source.absolute_episode ?? "-"}` : undefined],
      ["magnet", item?.magnet_uri],
    ]],
    ["File", [
      ["index", fileIndex >= 0 ? `${fileIndex}${fileCount ? ` / ${fileCount}` : ""}` : undefined], ["path", file?.path],
      ["size", file?.length], ["video", meta ? `${meta.video_codec} · ${meta.formatted_duration || meta.duration_sec}` : undefined],
    ]],
    ["Pipeline", [
      ["mode", remux ? "remux (forced)" : "direct"], ["time offset", timeOffset],
      ["audio track", audio ? `${audio.index} · ${audio.codec} · ${audio.language} · ${audio.title}` : String(audioTrack)],
      ["subtitle track", subtitle ? `${subtitle.index} · ${subtitle.codec} · ${subtitle.language} · ${subtitle.title}` : subtitleTrack === null ? "off" : String(subtitleTrack)],
      ["stream url", streamUrl && absolute(streamUrl)], ["subtitle url", subtitleUrl && absolute(subtitleUrl)],
    ]],
  ];

  async function copy() {
    const report = [
      ...sections.flatMap(([title, rows]) => [`## ${title}`, ...rows.filter(([, v]) => v !== undefined && v !== null && v !== "").map(([k, v]) => `${k}: ${v}`)]),
      "## Backend logs", trace ? `docker logs gazes-dev-backend-1 2>&1 | grep ${trace}` : "",
    ].join("\n");
    if (await copyText(report)) { setCopied(true); setTimeout(() => setCopied(false), 2000); }
  }

  if (!open) {
    return <button type="button" className="player-debug-toggle" onClick={() => setOpen(true)} aria-label="Debug"><Bug className="h-3.5 w-3.5" />{trace?.slice(0, 8)}</button>;
  }
  return (
    <aside className="player-debug" aria-label="Debug">
      <header>
        <strong>debug</strong>
        <button type="button" onClick={copy}>{copied ? "copied" : "copy report"}</button>
        <button type="button" aria-label="Close" onClick={() => setOpen(false)}><X className="h-3.5 w-3.5" /></button>
      </header>
      {sections.map(([title, rows]) => (
        <section key={title}>
          <h4>{title}</h4>
          <dl>
            {rows.filter(([, v]) => v !== undefined && v !== null && v !== "").map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{String(v)}</dd></div>)}
          </dl>
        </section>
      ))}
      {trace && <p className="player-debug-hint">docker logs gazes-dev-backend-1 2&gt;&amp;1 | grep {trace}</p>}
    </aside>
  );
}
