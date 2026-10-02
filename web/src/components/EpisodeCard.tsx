"use client";
import { useState } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { EpisodeInfo } from "@/types/api";

export function EpisodeCard({episode, href}: {episode: EpisodeInfo; href: string}) {
 const {t, locale}=useI18n();
 const [failedImage, setFailedImage]=useState<string|null>(null);
 const hasPreview=!!episode.thumbnail && failedImage!==episode.thumbnail;
 const title=/^(?:Épisode|Episode)\s+\d+$/i.test(episode.title.trim())?null:episode.title;
 const content=<>
  {hasPreview&&<div className="episode-preview">
   {/* eslint-disable-next-line @next/next/no-img-element */}
   <img src={episode.thumbnail} alt="" loading="lazy" onError={()=>setFailedImage(episode.thumbnail!)} />
  </div>}
  <div className="episode-copy"><div><h2>{t("Épisode")} {episode.episode_number}</h2>{title&&<p>{title}</p>}{episode.upcoming&&<p>{episode.airing_at?<time dateTime={new Date(episode.airing_at*1000).toISOString()}>{new Intl.DateTimeFormat(locale==="fr"?"fr-FR":"en-GB",{dateStyle:"medium",timeStyle:"short"}).format(new Date(episode.airing_at*1000))}</time>:t("Date à confirmer")}</p>}</div>{!episode.upcoming&&<Play size={20} aria-hidden="true" />}</div>
 </>;
 const className=`episode-card${episode.upcoming?" upcoming":""}${hasPreview?" has-preview":" compact-episode"}`;
 return episode.upcoming?<div className={className}>{content}</div>:<Link className={className} href={href}>{content}</Link>;
}
