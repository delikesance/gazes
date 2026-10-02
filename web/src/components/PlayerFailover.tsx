"use client";
import { Loader2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Scribble } from "./ui/Scribble";

export interface FailoverInfo {
  /** Sources already tried in this session, oldest first. */
  tried: { label: string; reason?: string }[];
  /** The source being connected right now. */
  current: string;
}

/** Shown while the player moves on to the next source after one fails. */
export function PlayerFailover({ info, onChangeSource }: { info: FailoverInfo; onChangeSource?: () => void }) {
  const { t } = useI18n();
  return (
    <div className="absolute inset-0 z-40 flex items-center justify-center bg-black/70 p-4" role="status" aria-live="polite">
      <div className="player-modal-card relative w-full max-w-[560px] overflow-hidden !p-8 sm:!p-9">
        <Scribble shape="a" width={260} rotate={-8} style={{ left: -80, top: -70 }} />
        <Scribble shape="b" width={300} rotate={6} style={{ right: -90, bottom: -70 }} />
        <div className="relative flex flex-col gap-5">
          <div className="flex flex-col gap-2.5">
            <span className="eyebrow">{t("Source indisponible")}</span>
            <h2 className="serif m-0 text-[32px] leading-[1.1] sm:text-4xl">{t("Cette source ne répond pas.")}</h2>
            <p className="m-0 text-sm leading-relaxed text-zinc-400">{t("Nous essayons automatiquement la suivante, sans perdre votre position.")}</p>
          </div>
          <ul className="m-0 flex list-none flex-col gap-1.5 p-0">
            {info.tried.map((source, index) => (
              <li key={index} className="player-source-row">
                <span className="font-mono text-zinc-500">{index + 1}</span>
                <span className="min-w-0 flex-1 truncate">{source.label}</span>
                <span className="max-w-[55%] truncate text-xs text-red-300" title={source.reason ? t(source.reason) : undefined}>{source.reason ? t(source.reason) : t("Sans réponse")}</span>
              </li>
            ))}
            <li className="player-source-row">
              <span className="font-mono text-zinc-500">{info.tried.length + 1}</span>
              <span className="min-w-0 flex-1 truncate">{info.current}</span>
              <span className="inline-flex items-center gap-1.5 text-xs" style={{ color: "var(--accent)" }}><Loader2 size={14} className="animate-spin" aria-hidden="true" />{t("Connexion…")}</span>
            </li>
          </ul>
          {onChangeSource && <button type="button" className="clay clay-secondary clay-lg" onClick={onChangeSource}>{t("Changer de source")}</button>}
        </div>
      </div>
    </div>
  );
}
