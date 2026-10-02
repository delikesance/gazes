"use client";
import { useWatchProgress, saveProgress } from "@/lib/watch-progress";
import { HeaderBreadcrumb } from "./HeaderBreadcrumb";
import { EpisodeCard } from "./EpisodeCard";
import { useI18n } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import dynamic from "next/dynamic";
import { AnimeCatalogItem, EpisodeSourcesResponse, Franchise } from "@/types/api";
import { getFranchise, getSeason, getSeasonSources } from "@/lib/api";
import { LazyImage } from "./ui/LazyImage";
import { AnimeHero } from "./AnimeHero";
const Player = dynamic(() => import("./AutoEpisodePlayer").then(m => m.AutoEpisodePlayer), {ssr:false});
interface Loaded {
  key: string;
  franchise: Franchise;
  season?: AnimeCatalogItem;
  sources?: EpisodeSourcesResponse;
}

interface AnimeBrowserProps {
  initialFranchise?: Franchise | null;
  initialSeason?: AnimeCatalogItem | null;
  initialSources?: EpisodeSourcesResponse | null;
  animeId?: number;
  seasonId?: number;
  episodeNumber?: number;
}

export function AnimeBrowser({
  initialFranchise = null,
  initialSeason = null,
  initialSources = null,
  animeId: propAnimeId,
  seasonId: propSeasonId,
  episodeNumber: propEpisodeNumber,
}: AnimeBrowserProps = {}) {
  const { t } = useI18n();
 const params = useParams<{animeId:string;seasonId?:string;episodeNumber?:string}>(); const router=useRouter();
 const id=Number(params?.animeId) || propAnimeId || 0, seasonId=Number(params?.seasonId) || propSeasonId || 0, ep=Number(params?.episodeNumber) || propEpisodeNumber || 0;
 const progress=useWatchProgress(seasonId);
 const key=`${id}/${seasonId}/${ep}`;
 const [loaded,setLoaded]=useState<Loaded|null>(() => {
   if (initialFranchise) {
     return { key: `${key}:0`, franchise: initialFranchise, season: initialSeason || undefined, sources: initialSources || undefined };
   }
   return null;
 });
 const [session]=useState(()=>initialSources?.playback_session_id || crypto.randomUUID());
 const [failure,setFailure]=useState<{key:string;message:string;reference?:string}|null>(null);
 const [retry,setRetry]=useState(0);
 const requestKey=`${key}:${retry}`;
 useEffect(() => {
  if (!id) return;
  let active = true;
  async function load() {
   try {
    const franchise=await getFranchise(id);
    if (!active) return;
    if (franchise.complete && franchise.id!==id) {router.replace(`/anime/${franchise.id}${seasonId?`/seasons/${seasonId}`:""}${ep?`/episodes/${ep}`:""}`);return;}
    const [season,sources]=await Promise.all([
     seasonId>0?getSeason(id,seasonId):Promise.resolve(undefined),
     ep>0?getSeasonSources(id,seasonId,ep,undefined,session):Promise.resolve(undefined),
    ]);
    if (active) {setLoaded({key:requestKey,franchise,season,sources});setFailure(null);}
   } catch(err) { if (active) setFailure({key:requestKey,message:err instanceof Error?err.message:"Impossible de charger les données.",reference:(err as any)?.diagnosticReference}); }
  }
  load();return()=>{active = false;};
 },[id,seasonId,ep,requestKey,router]);
 const current=loaded?.key===requestKey?loaded:loaded;
 const error=failure?.key===requestKey?failure.message:null;
 const reference=failure?.key===requestKey?failure.reference:undefined;
 const selected=current?.franchise.seasons.find((s: any)=>s.id===seasonId);
 const base=`/anime/${current?.franchise.id||id}`, seasonURL=`${base}/seasons/${seasonId}`;

 const heroTitle=ep?`${t(selected?.season_name || "Saison")} — ${t("Épisode")} ${ep}`:seasonId?t(selected?.season_name || current?.season?.display_title || "Saison"):current?.franchise.title || "Anime";
 const resume=progress&&current?.season?.episode_list?.some((episode: any)=>episode.episode_number===progress.episode&&!episode.upcoming)?progress:null;
 if(ep>0) return <main className="watch-page">
  {error?<div className="watch-message" role="alert"><p>{t(error)}</p>{reference&&<p>{t("Référence de diagnostic")} : <code>{reference}</code></p>}<button className="design-button" onClick={()=>setRetry(retry+1)}>{t("Réessayer")}</button><Link href={seasonURL}>{t("Voir les saisons")}</Link></div>:!current?<div className="watch-message" role="status"><p>{t("Chargement…")}</p><Link href={seasonURL}>{t("Voir les saisons")}</Link></div>:current.season&&current.sources?<Player pageMode key={`${key}:${retry}`} initialTime={resume?.episode===ep?resume.position:0} onProgress={(position:number,duration:number)=>saveProgress(seasonId,ep,position,duration)} sources={current.sources.sources} diagnosticSession={current.sources.playback_session_id||session} animeId={id} seasonId={seasonId} partial={current.sources.partial} onRetrySources={()=>setRetry(retry+1)} animeTitle={current.season.display_title} episodeNumber={ep} totalEpisodes={current.season.episodes} onClose={()=>router.push(seasonURL)} onPrevEpisode={ep>1?()=>router.push(`${seasonURL}/episodes/${ep-1}`):undefined} onNextEpisode={current.season.episode_list?.some((e: any)=>e.episode_number===ep+1&&!e.upcoming)?()=>router.push(`${seasonURL}/episodes/${ep+1}`):undefined}/>:null}
 </main>;
 return <main className="detail-page">
  <HeaderBreadcrumb><nav aria-label={t("Fil d’Ariane")} className="detail-breadcrumb"><Link href="/">{t("Catalogue")}</Link>{seasonId>0?<><span aria-hidden="true">›</span><Link href={base} title={current?.franchise.title}>{current?.franchise.title||t("Anime")}</Link><span aria-hidden="true">›</span>{ep>0?<Link href={seasonURL}>{selected?.season_number?t(`Saison ${selected.season_number}`):t("Saison")}</Link>:<span className="breadcrumb-current" aria-current="page">{selected?.season_number?t(`Saison ${selected.season_number}`):t("Saison")}</span>}</>:<><span aria-hidden="true">›</span><span className="breadcrumb-current" aria-current="page">{current?.franchise.title||t("Anime")}</span></>}{ep>0&&<><span aria-hidden="true">›</span><span className="breadcrumb-current" aria-current="page">{t("Épisode")} {ep}</span></>}</nav></HeaderBreadcrumb>
  {error?<div className="page-inset pt-24" role="alert">{t(error)} <button className="text-action" onClick={()=>setRetry(retry+1)}>{t("Réessayer")}</button></div>:!current?<div className="hero-skeleton" role="status"><p>{t("Chargement…")}</p><div /><div /></div>:<>
   <AnimeHero title={heroTitle} banner={current.season?.banner_image} poster={current.season?.poster_image || current.franchise.poster_image} episodes={current.season?.episodes} year={current.season?.season_year}>
    {!seasonId && <a className="design-button" href="#seasons">{t("Voir les saisons")}</a>}
    {seasonId>0&&!ep&&resume&&<Link className="design-button" href={`${seasonURL}/episodes/${resume.episode}`}>{t("Reprendre")} · {t("Épisode")} {resume.episode}</Link>}

    {seasonId>0&&!ep&&<Link className="design-button secondary-button" href={`${base}#seasons`}>{t("Voir les saisons")}</Link>}
   </AnimeHero>
   <div className="detail-content">
    {current.franchise.warning&&<p role="status" className="catalog-warning">{t(current.franchise.warning)} <button className="text-action" onClick={()=>setRetry(retry+1)}>{t("Réessayer")}</button></p>}
    {!seasonId && <>
     {current.franchise.description&&<p className="detail-description">{current.franchise.description}</p>}
     <div id="seasons">{([['main',t('Saisons')],['movies',t('Films')],['extras',t('Spéciaux et histoires annexes')]] as const).map(([group,title])=>{
      const entries=current.franchise.seasons.filter((s: any)=>s.group===group);
      return entries.length>0&&<section key={group} className="detail-section"><div className="section-heading"><h2>{t(title)}</h2><span className="heading-line" /></div><div className="season-grid">{entries.map((s: any)=><Link key={s.id} href={`${base}/seasons/${s.id}`} className="season-card"><LazyImage src={s.poster_image} alt={s.title} aspectRatio="" className="season-poster"/><h3>{t(s.season_name)}</h3><p>{s.season_year||t("Date inconnue")} · {s.episodes?t(s.episodes === 1 ? "{count} épisode" : "{count} épisodes", {count:s.episodes}):t("Nombre d’épisodes inconnu")}{s.status==="NOT_YET_RELEASED"?` ${t("· À venir")}`:""}</p></Link>)}</div></section>;
     })}</div>
    </>}
    {seasonId>0&&!ep&&<section id="episodes" className="detail-section"><div className="section-heading"><h2>{t("Épisodes")}</h2><span className="heading-line" /></div><div className="episode-grid">{current.season?.episode_list?.map((episode: any)=><EpisodeCard key={episode.episode_number} episode={episode} href={`${seasonURL}/episodes/${episode.episode_number}`} />)}</div>{!current.season?.episode_list?.length&&<p className="detail-notice">{t("La liste des épisodes n’est pas encore disponible.")}</p>}</section>}

   </div>
  </>}

 </main>;
}
