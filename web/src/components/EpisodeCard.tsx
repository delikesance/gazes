"use client";
import { useState } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { EpisodeInfo } from "@/types/api";

/** Release titles often look like "Episode 7 - The Small Blade - The Battle for Trost (3)": keep the episode name and the arc apart. */
export function splitEpisodeTitle(raw: string): { title: string | null; subtitle: string | null } {
 const parts = raw.trim().replace(/^(?:Épisode|Episode)\s+\d+\s*[-–:]?\s*/i, "").split(/\s+[-–]\s+/).map((part) => part.trim()).filter(Boolean);
 if (!parts.length) return { title: null, subtitle: null };
 return { title: parts[0], subtitle: parts.length > 1 ? parts.slice(1).join(" · ") : null };
}

export function EpisodeCard({episode, href, current = false}: {episode: EpisodeInfo; href: string; current?: boolean}) {
 const {t, locale}=useI18n();
 const [failedImage, setFailedImage]=useState<string|null>(null);
 const hasPreview=!!episode.thumbnail && failedImage!==episode.thumbnail;
 const {title, subtitle}=splitEpisodeTitle(episode.title);
 const content=<>
  <span className="episode-number">{episode.episode_number}</span>
  {hasPreview&&<div className="episode-preview">
   {/* eslint-disable-next-line @next/next/no-img-element */}
   <img src={episode.thumbnail} alt="" loading="lazy" onError={()=>setFailedImage(episode.thumbnail!)} />
  </div>}
  <div className="episode-copy"><div>
   <h2>{title||`${t("Épisode")} ${episode.episode_number}`}</h2>
   {subtitle&&<p>{subtitle}</p>}
   {current&&<p className="episode-status">{t("En cours de visionnage")}</p>}
   {episode.upcoming&&<p>{episode.airing_at?<time dateTime={new Date(episode.airing_at*1000).toISOString()}>{new Intl.DateTimeFormat(locale==="fr"?"fr-FR":"en-GB",{dateStyle:"medium",timeStyle:"short"}).format(new Date(episode.airing_at*1000))}</time>:t("Date à confirmer")}</p>}
  </div>{!episode.upcoming&&<span className="episode-play-button"><Play size={14} aria-hidden="true" /></span>}</div>
 </>;
 const className=`episode-card${episode.upcoming?" upcoming":""}${current?" current":""}${hasPreview?" has-preview":" compact-episode"}`;
 return episode.upcoming?<div className={className}>{content}</div>:<Link className={className} href={href} aria-label={`${t("Épisode")} ${episode.episode_number}${title?` — ${title}`:""}`}>{content}</Link>;
}
