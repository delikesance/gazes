"use client";
import { randomId } from "@/lib/random-id";
import { useI18n } from "@/lib/i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { EpisodeInfo, EpisodeSource } from '@/types/api';
import { diagnosticEvent } from "@/lib/diagnostics";
import { loadTorrent } from "@/lib/api";
import { playbackSources } from '@/lib/playback-sources';
import { VideoPlayerModal } from './VideoPlayerModal';
import { EpisodeSourceSelectorModal } from './EpisodeSourceSelectorModal';
import type { FailoverInfo } from './PlayerFailover';

interface Props {
 sources: EpisodeSource[];
 diagnosticSession?:string;
 animeId?:number;
 seasonId?:number;
 partial?:boolean;
 onRetrySources?:()=>void;
 animeTitle: string;
 episodeNumber: number;
 pageMode?: boolean;
 totalEpisodes?: number;
 initialTime?: number;
 onProgress?: (position:number,duration:number)=>void;
 onClose: () => void;
 onNextEpisode?: () => void;
 onPrevEpisode?: () => void;
 episodes?: EpisodeInfo[];
 onSelectEpisode?: (episode: number) => void;
}

export function AutoEpisodePlayer({ sources, diagnosticSession, animeId, seasonId, partial, onRetrySources, ...props }: Props) {
  const { t } = useI18n();
 const [fallbackSession]=useState(()=>randomId());
 const session=diagnosticSession||fallbackSession;
 const candidates = useMemo(() => playbackSources(sources), [sources]);
 const [attempt, setAttempt] = useState({ index: 0, position: props.initialTime || 0, reason: '', id:randomId() });
 const [choosingSource, setChoosingSource] = useState(false);
 const [tried, setTried] = useState<{ label: string }[]>([]);
 const sourceLabel = (s?: EpisodeSource) => s ? [s.release_group || s.provider, s.quality].filter(Boolean).join(' · ') || s.title : '';
 const positionRef = useRef(props.initialTime || 0);
 const source = candidates[attempt.index];
 const diagnostic=useMemo(()=>({playback_session_id:session,attempt_id:attempt.id,anime_id:animeId?String(animeId):undefined,season_id:seasonId?String(seasonId):undefined,episode:String(props.episodeNumber),infohash:source?.info_hash}),[session,attempt.id,animeId,seasonId,props.episodeNumber,source?.info_hash]);
 useEffect(()=>{diagnosticEvent(diagnostic,source?'playback.attempt':'playback.exhausted',{source_count:candidates.length,partial:Boolean(partial)});},[diagnostic,source,candidates.length,partial]);
 useEffect(()=>{
  const error=(event:ErrorEvent)=>diagnosticEvent(diagnostic,'browser.error',{reason:event.message,error_code:'javascript_error'});
  const rejection=(event:PromiseRejectionEvent)=>diagnosticEvent(diagnostic,'browser.error',{reason:String(event.reason?.message||event.reason),error_code:'unhandled_rejection'});
  window.addEventListener('error',error);window.addEventListener('unhandledrejection',rejection);
  return()=>{window.removeEventListener('error',error);window.removeEventListener('unhandledrejection',rejection);};
 },[diagnostic]);
 const failed = useCallback((failure: { reason: string; position: number }) => {
  setTried(list => [...list, { label: sourceLabel(candidates[attempt.index]) }]);
  setAttempt(current => current.id === attempt.id
   ? { index: current.index + 1, position: Math.max(0, failure.position), reason: failure.reason, id:randomId() }
   : current);
 }, [attempt.id, attempt.index, candidates]); // eslint-disable-line react-hooks/exhaustive-deps
 // Warm the next candidates' metadata in the background: a failing source then hands over to a ready one.
 useEffect(()=>{
  const upcoming=candidates.slice(attempt.index+1,attempt.index+3);
  if(!upcoming.length)return;
  const controller=new AbortController();
  const timer=setTimeout(async()=>{
   for(const next of upcoming){
    if(controller.signal.aborted)return;
    try{await loadTorrent(next.magnet_uri,{metadataOnly:true,prewarm:true,signal:controller.signal});}catch{/* best effort */}
   }
  },3000);
  return()=>{clearTimeout(timer);controller.abort();};
 },[candidates,attempt.index]);
 if (!source) return <div className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center p-6">
  <div role="alert" className="max-w-lg space-y-4 text-center text-zinc-100">
   <p>{t(candidates.length?"Toutes les tentatives de lecture ont échoué.":partial?"La recherche de torrents est incomplète. Réessayez.":"Aucun torrent ne correspond à cet épisode.")}</p>
   <p className="text-xs text-zinc-400">{t("Référence de diagnostic")} : <code>{session}</code></p>
   {attempt.reason && <p className="text-sm text-zinc-400">{t(attempt.reason)}</p>}
   <div className="flex justify-center gap-4">
    {<button className="rounded-full bg-white text-black px-4 py-2" onClick={() => candidates.length?setAttempt({ index: 0, position: attempt.position, reason: '', id:randomId() }):onRetrySources?.()}>{t("Réessayer")}</button>}
    <button className="underline" onClick={props.onClose}>{t("Fermer")}</button>
   </div>
  </div>
 </div>;
 const picker = <EpisodeSourceSelectorModal animeTitle={props.animeTitle} episodeNumber={props.episodeNumber}
  sourcesData={{anime_title:props.animeTitle,episode_number:props.episodeNumber,total_sources:candidates.length,french_sources:candidates.filter(s=>s.is_french).length,sources:candidates,partial}}
  currentSourceHash={source?.info_hash} isLoading={false} isOpen={choosingSource}
  onClose={()=>setChoosingSource(false)} onSelectSource={selected=>{
   const index=candidates.findIndex(candidate=>candidate.info_hash===selected.info_hash);
   if(index<0)return;
   setChoosingSource(false);
   if(index===attempt.index)return;
   setTried([]);
   setAttempt({index,position:positionRef.current,reason:'',id:randomId()});
  }} />;
 return <VideoPlayerModal key={attempt.id} {...props}
  onChangeSource={()=>setChoosingSource(true)} sourcePicker={picker}
  onProgress={(position,duration)=>{positionRef.current=position;props.onProgress?.(position,duration);}}
  failover={tried.length?({tried,current:sourceLabel(source)} satisfies FailoverInfo):undefined}
  debugAttempt={{index:attempt.index,count:candidates.length,tried:tried.map(entry=>entry.label)}} item={source} initialTime={attempt.position} onPlaybackFailure={failed} diagnostic={diagnostic}

 />;
}
