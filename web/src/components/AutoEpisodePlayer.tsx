"use client";
import { randomId } from "@/lib/random-id";
import { useI18n } from "@/lib/i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { EpisodeInfo, EpisodeSource, LibraryCopy } from '@/types/api';
import { diagnosticEvent } from "@/lib/diagnostics";
import { loadTorrent, getSeasonSources, getLibraryCopies, registerLibraryCopy } from "@/lib/api";
import { libraryLang, lookupWithin, playerWaitState, prewarmTargets, withLibraryCandidates } from '@/lib/library';
import { useAuth } from './AuthProvider';
import { playbackSources, extendPlaybackSources } from '@/lib/playback-sources';
import { VideoPlayerModal } from './VideoPlayerModal';
import { EpisodeSourceSelectorModal } from './EpisodeSourceSelectorModal';
import type { FailoverInfo } from './PlayerFailover';
import { usePlaybackEngine } from '@/lib/use-playback-engine';

interface Props {
 sources: EpisodeSource[];
 /** The source search has not answered yet: the library copies in initialLibraryCopies play meanwhile. */
 sourcesPending?: boolean;
 /** The sources request failed (a library copy may still have played); retrying reloads it. */
 sourcesFailed?: boolean;
 /** Copies the caller already looked up (possibly empty); the player then skips its own lookup. */
 initialLibraryCopies?: LibraryCopy[];
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
 fallbackThumbnail?: string;
 onSelectEpisode?: (episode: number) => void;
}

const LIBRARY_LOOKUP_MS = 1500;
const isTypeSupported = (mime: string) => typeof MediaSource !== 'undefined' && typeof MediaSource.isTypeSupported === 'function' && MediaSource.isTypeSupported(mime);

/** Rank pending fallbacks without disturbing attempted ones; pending library copies keep their place before the first same-language torrent. */
function mergeSources(list: EpisodeSource[], discovered: EpisodeSource[], active: number, copies: LibraryCopy[], base: Partial<EpisodeSource>): EpisodeSource[] {
 const fixed = list.slice(0, active + 1);
 const used = new Set(fixed.map(s => s.info_hash));
 const extended = extendPlaybackSources([...fixed, ...list.slice(fixed.length).filter(s => !s.library)], discovered, active);
 const tail = withLibraryCandidates(extended.slice(fixed.length), copies.filter(c => !used.has(c.stream_id)), isTypeSupported, base);
 return [...fixed, ...tail];
}

export function AutoEpisodePlayer({ sources, sourcesPending, sourcesFailed, initialLibraryCopies, diagnosticSession, animeId, seasonId, partial, onRetrySources, ...props }: Props) {
  const { t } = useI18n();
 const engine = usePlaybackEngine();
 const pausedRef = useRef(false);
 const [fallbackSession]=useState(()=>randomId());
 const session=diagnosticSession||fallbackSession;
 const { user } = useAuth();
 const libraryBase = useMemo(() => ({ title: props.animeTitle, episode_number: props.episodeNumber }), [props.animeTitle, props.episodeNumber]);
 const [candidates, setCandidates] = useState(() => initialLibraryCopies ? mergeSources(playbackSources(sources), [], -1, initialLibraryCopies, libraryBase) : playbackSources(sources));
 // Without initialLibraryCopies the first attempt waits for the library lookup, at most LIBRARY_LOOKUP_MS; a later answer is ignored.
 const [libraryReady, setLibraryReady] = useState(!seasonId || !!initialLibraryCopies);
 const libraryReadyRef = useRef(!seasonId || !!initialLibraryCopies);
 const libraryCopiesRef = useRef<LibraryCopy[]>(initialLibraryCopies ?? []);
 const registeredRef = useRef(new Set<string>());
 const [attempt, setAttempt] = useState({ index: 0, position: props.initialTime || 0, reason: '', id:randomId() });
 const attemptRef = useRef(attempt);
 attemptRef.current = attempt;
 const [discoveryState, setDiscoveryState] = useState<'idle'|'loading'|'done'|'failed'>('idle');
 const discoveryController = useRef<AbortController | null>(null);
 const [choosingSource, setChoosingSource] = useState(false);
 const [tried, setTried] = useState<{ label: string; reason?: string }[]>([]);
 const sourceLabel = (s?: EpisodeSource) => s ? [s.release_group || s.provider, s.quality].filter(Boolean).join(' · ') || s.title : '';
 const positionRef = useRef(props.initialTime || 0);
 const source = candidates[attempt.index];
 const diagnostic=useMemo(()=>({playback_session_id:session,attempt_id:attempt.id,anime_id:animeId?String(animeId):undefined,season_id:seasonId?String(seasonId):undefined,episode:String(props.episodeNumber),infohash:source?.info_hash}),[session,attempt.id,animeId,seasonId,props.episodeNumber,source?.info_hash]);
 useEffect(()=>{const active=libraryReadyRef.current?attemptRef.current.index:-1;setCandidates(list=>mergeSources(list,sources,active,libraryCopiesRef.current,libraryBase));},[sources,libraryBase]);
 useEffect(()=>{
  if(!seasonId||initialLibraryCopies)return;
  let cancelled=false;
  void lookupWithin(LIBRARY_LOOKUP_MS,signal=>getLibraryCopies(seasonId,props.episodeNumber,signal)).then(({copies})=>{
   if(cancelled||libraryReadyRef.current)return;
   libraryCopiesRef.current=copies;
   setCandidates(list=>mergeSources(list,[],-1,copies,libraryBase));
  }).catch(()=>{}).finally(()=>{
   // The gate must open whatever happened above; lookupWithin's own timer bounds the wait.
   if(cancelled||libraryReadyRef.current)return;
   libraryReadyRef.current=true;setLibraryReady(true);
  });
  return()=>{cancelled=true;};
 },[seasonId,props.episodeNumber,libraryBase,initialLibraryCopies]);
 useEffect(()=>()=>discoveryController.current?.abort(),[]);
 useEffect(()=>{
  if (!partial || !animeId || !seasonId || discoveryState!=='idle' || (attempt.index===0 && source)) return;
  const controller=new AbortController();discoveryController.current=controller;setDiscoveryState('loading');
  diagnosticEvent(diagnostic,'playback.discovery_started',{discovery:'full'});
  void getSeasonSources(animeId,seasonId,props.episodeNumber,controller.signal,session,'full').then(data=>{
   if(controller.signal.aborted)return;
   const active=attemptRef.current.index;
   setCandidates(list=>mergeSources(list,data.sources,active,libraryCopiesRef.current,libraryBase));
   setDiscoveryState('done');
   diagnosticEvent(diagnostic,'playback.discovery_completed',{source_count:data.sources.length,partial:Boolean(data.partial)});
  }).catch(()=>{if(!controller.signal.aborted){setDiscoveryState('failed');diagnosticEvent(diagnostic,'playback.discovery_failed');}});
 },[partial,animeId,seasonId,discoveryState,attempt.index,source,props.episodeNumber,session,diagnostic,libraryBase]);
 useEffect(()=>{diagnosticEvent(diagnostic,source?'playback.attempt':'playback.exhausted',{source_count:candidates.length,partial:Boolean(partial)});},[diagnostic,source,candidates.length,partial]);
 useEffect(()=>{
  const error=(event:ErrorEvent)=>diagnosticEvent(diagnostic,'browser.error',{reason:event.message,error_code:'javascript_error'});
  const rejection=(event:PromiseRejectionEvent)=>diagnosticEvent(diagnostic,'browser.error',{reason:String(event.reason?.message||event.reason),error_code:'unhandled_rejection'});
  window.addEventListener('error',error);window.addEventListener('unhandledrejection',rejection);
  return()=>{window.removeEventListener('error',error);window.removeEventListener('unhandledrejection',rejection);};
 },[diagnostic]);
 const failed = useCallback((failure: { reason: string; position: number }) => {
  setTried(list => [...list, { label: sourceLabel(candidates[attempt.index]), reason: failure.reason }]);
  setAttempt(current => current.id === attempt.id
   ? { index: current.index + 1, position: Math.max(0, failure.position), reason: failure.reason, id:randomId() }
   : current);
 }, [attempt.id, attempt.index, candidates]); // eslint-disable-line react-hooks/exhaustive-deps
 // French audio is selected within VideoPlayerModal when available; missing or
 // unconfirmed French audio must never reject an otherwise playable source.
 // Warm the next candidates' metadata in the background: a failing source then hands over to a ready one.
 useEffect(()=>{
  const upcoming=prewarmTargets(candidates,attempt.index);
  if(!upcoming.length)return;
  const controller=new AbortController();
  const timer=setTimeout(()=>{
   void Promise.allSettled(upcoming.map(next=>loadTorrent(next.magnet_uri,{metadataOnly:true,prewarm:true,signal:controller.signal})));
  },500);
  return()=>{clearTimeout(timer);controller.abort();};
 },[candidates,attempt.index]);
 if (!libraryReady) return <div role="status" className="fixed inset-0 z-50 bg-black/90" />;
 if (!source && partial && animeId && seasonId && (discoveryState==='idle'||discoveryState==='loading')) return <div role="status" className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center p-6 text-zinc-100">{t("Recherche de sources supplémentaires…")}</div>;
 if (playerWaitState({ hasSource: Boolean(source), sourcesState: sourcesPending ? 'pending' : 'loaded' }) === 'pending') return <div role="status" className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center p-6 text-zinc-100">{t("Recherche de sources supplémentaires…")}</div>;
 if (!source) return <div className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center p-6">
  <div role="alert" className="max-w-lg space-y-4 text-center text-zinc-100">
   <p>{t(candidates.length?"Toutes les tentatives de lecture ont échoué.":partial?"La recherche de torrents est incomplète. Réessayez.":"Aucun torrent ne correspond à cet épisode.")}</p>
   <p className="text-xs text-zinc-400">{t("Référence de diagnostic")} : <code>{session}</code></p>
   {sourcesFailed && <p className="text-sm text-zinc-400">{t("La recherche de sources a échoué.")}</p>}
   {attempt.reason && <p className="text-sm text-zinc-400">{t(attempt.reason)}</p>}
   <div className="flex justify-center gap-4">
    {<button className="rounded-full bg-white text-black px-4 py-2" onClick={() => candidates.length&&!sourcesFailed?setAttempt({ index: 0, position: attempt.position, reason: '', id:randomId() }):onRetrySources?.()}>{t("Réessayer")}</button>}
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
 const onFileResolved=(infoHash:string,fileIndex:number)=>{
  const played=candidates.find(candidate=>candidate.info_hash.toLowerCase()===infoHash.toLowerCase());
  const lang=played&&!played.library?libraryLang(played):null;
  if(!user||!lang||!seasonId||!animeId||!played)return;
  const key=`${seasonId}:${props.episodeNumber}:${lang}:${infoHash}:${fileIndex}`;
  if(registeredRef.current.has(key))return;
  registeredRef.current.add(key);
  void registerLibraryCopy(seasonId,props.episodeNumber,lang,{info_hash:infoHash,file_index:fileIndex,release_name:played.title,anime_id:animeId,title:props.animeTitle});
 };
 return <VideoPlayerModal key={engine === 'hls' ? 'hls-player' : attempt.id} {...props}
  initialPaused={pausedRef.current} onPlaybackIntent={playing=>{pausedRef.current=!playing;}}
  onChangeSource={()=>setChoosingSource(true)} sourcePicker={picker}
  onProgress={(position,duration)=>{positionRef.current=position;props.onProgress?.(position,duration);}}
  failover={tried.length?({tried,current:sourceLabel(source)} satisfies FailoverInfo):undefined}
  onFileResolved={onFileResolved} debugAttempt={{index:attempt.index,count:candidates.length,tried:tried.map(entry=>entry.label)}} item={source} initialTime={attempt.position} onPlaybackFailure={failed} diagnostic={diagnostic}

 />;
}
