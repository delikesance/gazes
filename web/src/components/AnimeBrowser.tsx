"use client";
import { ErrorAlert } from "./ErrorAlert";
import { errorCode } from "@/lib/error-code";
import { randomId } from "@/lib/random-id";
import { useWatchProgress, saveProgress } from "@/lib/watch-progress";
import { canGoBackInApp } from "@/lib/navigation-history";
import { BackButton } from "./BackButton";
import { CatalogCrumb } from "./CatalogCrumb";
import { HeaderBreadcrumb } from "./HeaderBreadcrumb";
import { useI18n } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import dynamic from "next/dynamic";
import { AnimeCatalogItem, EpisodeSourcesResponse, Franchise, LibraryCopy } from "@/types/api";
import { getFranchise, getSeason, getSeasonSources, getLibraryCopies } from "@/lib/api";
import { currentFor, loadWatchData, lookupWithin, type SourcesState } from "@/lib/library";
import { SeriesPage } from "./SeriesPage";
import { SeasonPage } from "./SeasonPage";
import { WatchLoading } from "./WatchLoading";
const Player = dynamic(() => import("./AutoEpisodePlayer").then(m => m.AutoEpisodePlayer), {ssr:false});
interface Loaded {
  key: string;
  franchise: Franchise;
  season?: AnimeCatalogItem;
  sources?: EpisodeSourcesResponse;
  /** Outcome of the /sources request: 'pending' only while it is really in flight. */
  sourcesState: SourcesState;
  /** Library copies already looked up for this episode; undefined when the player must look them up itself. */
  libraryCopies?: LibraryCopy[];
}

const LIBRARY_LOOKUP_MS = 1500;
const NO_SOURCES: never[] = [];

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
 // `?t=<seconds>` from a shared link applies to the episode it was opened on only.
 const [shared]=useState(()=>({key, t: typeof window==="undefined"?0:Math.max(0,Math.floor(Number(new URLSearchParams(window.location.search).get("t")))||0)}));
 const sharedStart=shared.key===key?shared.t:0;
 const [loaded,setLoaded]=useState<Loaded|null>(() => {
   if (initialFranchise) {
     return { key: `${key}:0`, franchise: initialFranchise, season: initialSeason || undefined, sources: initialSources || undefined, sourcesState: initialSources ? 'loaded' : 'pending' };
   }
   return null;
 });
 const [session]=useState(()=>initialSources?.playback_session_id || randomId());
 const [failure,setFailure]=useState<{key:string;message:string;reference?:string;code?:string}|null>(null);
 const [retry,setRetry]=useState(0);
 const requestKey=`${key}:${retry}`;
 useEffect(() => {
  if (!id) return;
  let active = true;
  let redirected = false;
  async function load() {
   try {
    // The library lookup runs next to the source search (bounded by LIBRARY_LOOKUP_MS) so that a local copy
    // can start playing without waiting for the indexers.
    await loadWatchData({
     franchise:getFranchise(id),
     season:seasonId>0?getSeason(id,seasonId):Promise.resolve(undefined),
     sources:ep>0?getSeasonSources(id,seasonId,ep,undefined,session):Promise.resolve(undefined),
     copies:seasonId>0&&ep>0?lookupWithin(LIBRARY_LOOKUP_MS,signal=>getLibraryCopies(seasonId,ep,signal)).then(r=>r.copies):Promise.resolve([]),
    },{
     ready:({franchise,season,copies,sources,sourcesState})=>{
      if (!active) return;
      if (franchise.complete && franchise.id!==id) {redirected=true;router.replace(`/anime/${franchise.id}${seasonId?`/seasons/${seasonId}`:""}${ep?`/episodes/${ep}`:""}`);return;}
      setLoaded({key:requestKey,franchise,season,sources,sourcesState,libraryCopies:copies});setFailure(null);
     },
     sources:sources=>{
      if (!active||redirected) return;
      setLoaded(prev=>prev&&prev.key===requestKey?{...prev,sources,sourcesState:'loaded'}:prev);
     },
     sourcesFailed:()=>{
      if (!active||redirected) return;
      setLoaded(prev=>prev&&prev.key===requestKey?{...prev,sourcesState:'failed'}:prev);
     },
    });
   } catch(err) { if (active) setFailure({key:requestKey,message:err instanceof Error?err.message:"Impossible de charger les données.",reference:(err as {diagnosticReference?:string}|null)?.diagnosticReference,code:errorCode(err,"SRC")}); }
  }
  load();return()=>{active = false;};
 },[id,seasonId,ep,requestKey,router]);
 const current=currentFor(loaded,requestKey);
 const error=failure?.key===requestKey?failure.message:null;
 const reference=failure?.key===requestKey?failure.reference:undefined;
 const code=failure?.key===requestKey?failure.code:undefined;
 const selected=current?.franchise.seasons.find(s=>s.id===seasonId);
 const base=`/anime/${current?.franchise.id||id}`, seasonURL=`${base}/seasons/${seasonId}`;

 const resume=progress&&current?.season?.episode_list?.some(episode=>episode.episode_number===progress.episode&&!episode.upcoming)?progress:null;
 if(ep>0) return <main className="watch-page">
  {error?<div className="watch-message"><ErrorAlert message={error} code={code} reference={reference} onRetry={()=>setRetry(retry+1)} /><Link href={seasonURL}>{t("Voir les saisons")}</Link></div>:!current?<WatchLoading episode={ep} backHref={seasonURL} />:current.season&&(current.sources||current.libraryCopies?.length)?<Player pageMode key={`${key}:${retry}`} initialTime={sharedStart>0?sharedStart:resume?.episode===ep?resume.position:0} onProgress={(position:number,duration:number,tracks?:{audioLang?:string;subLang?:string})=>saveProgress(seasonId,ep,position,duration,{animeId:id,title:current.season?.display_title,genres:current.season?.genres,format:current.season?.format,...tracks})} sources={current.sources?.sources??NO_SOURCES} sourcesPending={current.sourcesState==='pending'} sourcesFailed={current.sourcesState==='failed'} initialLibraryCopies={current.libraryCopies} diagnosticSession={current.sources?.playback_session_id||session} animeId={id} seasonId={seasonId} partial={current.sources?.partial} onRetrySources={()=>setRetry(retry+1)} animeTitle={current.season.display_title} episodeNumber={ep} totalEpisodes={current.season.episodes} episodes={current.season.episode_list} fallbackThumbnail={current.season.banner_image||current.season.poster_image} onSelectEpisode={(n:number)=>router.replace(`${seasonURL}/episodes/${n}`)} onClose={()=>canGoBackInApp()?router.back():router.push(seasonURL)} onPrevEpisode={ep>1?()=>router.replace(`${seasonURL}/episodes/${ep-1}`):undefined} onNextEpisode={current.season.episode_list?.some(e=>e.episode_number===ep+1&&!e.upcoming)?()=>router.replace(`${seasonURL}/episodes/${ep+1}`):undefined}/>:<WatchLoading episode={ep} backHref={seasonURL} />}
 </main>;
 return <main className="detail-page">
  <HeaderBreadcrumb><nav aria-label={t("Fil d’Ariane")} className="detail-breadcrumb"><BackButton fallback={seasonId>0?base:"/"} label={seasonId>0?(current?.franchise.title||t("Anime")):t("Catalogue")} /><CatalogCrumb />{seasonId>0?<><span aria-hidden="true">›</span><Link href={base} title={current?.franchise.title}>{current?.franchise.title||t("Anime")}</Link><span aria-hidden="true">›</span>{ep>0?<Link href={seasonURL}>{selected?.season_number?t(`Saison ${selected.season_number}`):t("Saison")}</Link>:<span className="breadcrumb-current" aria-current="page">{selected?.season_number?t(`Saison ${selected.season_number}`):t("Saison")}</span>}</>:<><span aria-hidden="true">›</span><span className="breadcrumb-current" aria-current="page">{current?.franchise.title||t("Anime")}</span></>}{ep>0&&<><span aria-hidden="true">›</span><span className="breadcrumb-current" aria-current="page">{t("Épisode")} {ep}</span></>}</nav></HeaderBreadcrumb>
  {error?<div className="page-inset pt-24"><ErrorAlert message={error} code={code} reference={reference} onRetry={()=>setRetry(retry+1)} /></div>:!current?<div className="hero-skeleton" role="status"><p>{t("Chargement…")}</p><div /><div /></div>:<>
   <div className="detail-content">
    {!seasonId&&<SeriesPage franchise={current.franchise} base={base} warning={current.franchise.warning&&<p role="status" className="catalog-warning">{t(current.franchise.warning)} <button className="text-action" onClick={()=>setRetry(retry+1)}>{t("Réessayer")}</button></p>} />}
    {seasonId>0&&<SeasonPage franchise={current.franchise} season={current.season} seasonId={seasonId} base={base} resume={resume} />}
   </div>
  </>}

 </main>;
}
