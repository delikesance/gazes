"use client";
import { useState } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { episodePreviewUrl } from "@/lib/api";
import type { EpisodeInfo } from "@/types/api";

/** Release titles often look like "Episode 7 - The Small Blade - The Battle for Trost (3)": keep the episode name and the arc apart. */
export function splitEpisodeTitle(raw: string): { title: string | null; subtitle: string | null } {
 const parts = raw.trim().replace(/^(?:Épisode|Episode)\s+\d+\s*[-–:]?\s*/i, "").split(/\s+[-–]\s+/).map((part) => part.trim()).filter(Boolean);
 if (!parts.length) return { title: null, subtitle: null };
 return { title: parts[0], subtitle: parts.length > 1 ? parts.slice(1).join(" · ") : null };
}

export function EpisodeCard({episode, href, current = false, watched = false, seasonId}: {episode: EpisodeInfo; href: string; current?: boolean; watched?: boolean; seasonId?: number}) {
 const {t, locale}=useI18n();
 const [failedImage, setFailedImage]=useState<string|null>(null);
 // Without a provider still, show the frame cut from the file when the episode was first played (404 until then).
 const previewSrc=episode.thumbnail||(seasonId&&!episode.upcoming?episodePreviewUrl(seasonId,episode.episode_number):undefined);
 const hasPreview=!!previewSrc && failedImage!==previewSrc;
 const {title, subtitle}=splitEpisodeTitle(episode.title);
 const content=<>
  <span className="episode-number">{episode.episode_number}</span>
  {hasPreview&&<div className="episode-preview">
   {/* eslint-disable-next-line @next/next/no-img-element */}
   <img src={previewSrc} alt="" loading="lazy" onError={()=>setFailedImage(previewSrc!)} ref={(img)=>{ if(img&&img.complete&&img.naturalWidth===0) setFailedImage(previewSrc!); }} />
  </div>}
  <div className="episode-copy"><div>
   <h2>{title||`${t("Épisode")} ${episode.episode_number}`}</h2>
   {subtitle&&<p>{subtitle}</p>}
   {current&&<p className="episode-status">{t("En cours de visionnage")}</p>}
   {watched&&!current&&<p className="episode-seen">{t("Vu")}</p>}
   {episode.upcoming&&<p>{episode.airing_at?<time dateTime={new Date(episode.airing_at*1000).toISOString()}>{new Intl.DateTimeFormat(locale==="fr"?"fr-FR":"en-GB",{dateStyle:"medium",timeStyle:"short"}).format(new Date(episode.airing_at*1000))}</time>:t("Date à confirmer")}</p>}
  </div>{!episode.upcoming&&<span className="episode-play-button"><Play size={14} aria-hidden="true" /></span>}</div>
 </>;
 const className=`episode-card${episode.upcoming?" upcoming":""}${current?" current":""}${watched?" watched":""}${hasPreview?" has-preview":" compact-episode"}`;
 return episode.upcoming?<div className={className}>{content}</div>:<Link className={className} href={href} aria-label={`${t("Épisode")} ${episode.episode_number}${title?` — ${title}`:""}`}>{content}</Link>;
}
