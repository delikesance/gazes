"use client";
import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { formatBytes } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { useI18n } from "@/lib/i18n";
import { formatTime } from "@/lib/player-state";
import type { FileInfo, SwarmStats, TorrentItem } from "@/types/api";

interface PlayerDetailsProps {
  item: TorrentItem;
  stats: SwarmStats | null;
  totalDuration: number;
  /** Shown while the duration is unknown. */
  formattedDuration?: string;
  fileSize: number;
  /** The pack's files for manual switching; absent when the source is picked automatically. */
  files?: FileInfo[];
  selectedFileIdx: number;
  onSelectFile: (index: number) => void;
  forceRemux: boolean;
  onForceRemuxChange: (remux: boolean) => void;
}

/** Below the video: swarm and file telemetry, the file switcher, remux toggle and magnet copy. */
export function PlayerDetails({ item, stats, totalDuration, formattedDuration, fileSize, files, selectedFileIdx, onSelectFile, forceRemux, onForceRemuxChange }: PlayerDetailsProps) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const handleCopyMagnet = () => {
    void copyText(item.magnet_uri);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <>
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
              {totalDuration > 0 ? formatTime(totalDuration) : formattedDuration || t("Detecting...")}
            </p>
          </div>

          {/* File Size */}
          <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-2.5">
            <p className="text-[11px] text-zinc-400">{t("File Size")}</p>
            <p className="font-mono text-zinc-200 font-medium mt-0.5">
              {formatBytes(fileSize)}
            </p>
          </div>
        </div>
      </div>

      {/* File Selector & Controls */}
      <div className="player-advanced p-4 space-y-3">
        {files && files.length > 1 && (
          <div>
            <label className="block text-xs text-zinc-400 mb-1 font-mono">
              {t("Episode / File (")}{files.length} {t("items):")}</label>
            <select
              value={selectedFileIdx}
              onChange={(e) => onSelectFile(parseInt(e.target.value, 10))}
              className="w-full rounded-full border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-200 focus:border-zinc-500 focus:outline-none font-mono"
            >
              {files.map((file) => (
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
              onChange={(e) => onForceRemuxChange(e.target.checked)}
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
    </>
  );
}
