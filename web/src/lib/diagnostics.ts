// Explicit per-player context avoids attributing concurrent players to each other.
export interface PlaybackDiagnostic {
 playback_session_id: string;
 attempt_id?: string;
 anime_id?: string;
 season_id?: string;
 episode?: string;
 infohash?: string;
}
export function diagnosticHeaders(context?: PlaybackDiagnostic): Record<string,string> {
 if (!context) return {};
 const h:Record<string,string>={'X-Playback-Session-ID':context.playback_session_id};
 for(const [field,header] of [['attempt_id','X-Playback-Attempt-ID'],['anime_id','X-Playback-Anime-ID'],['season_id','X-Playback-Season-ID'],['episode','X-Playback-Episode']] as const) if(context[field])h[header]=context[field]!;
 return h;
}
export function diagnosticURL(url:string,context?:PlaybackDiagnostic):string {
 if(!context)return url;
 const query=new URLSearchParams();for(const key of ['playback_session_id','attempt_id','anime_id','season_id','episode'] as const)if(context[key])query.set(key,context[key]!);
 return `${url}${url.includes('?')?'&':'?'}${query}`;
}
const queued:Record<string,unknown>[]=[];
let timer:ReturnType<typeof setTimeout>|undefined;
function flush(){
 timer=undefined;if(typeof window==='undefined'||!queued.length)return;
 const events=queued.splice(0,20);
 const base=process.env.NEXT_PUBLIC_API_BASE || '/api/v1';
 void fetch(`${base}/diagnostics/events`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({events}),keepalive:true}).catch(()=>{});
 if(queued.length)timer=setTimeout(flush,500);
}
export function diagnosticEvent(context:PlaybackDiagnostic|undefined,event:string,attributes:Record<string,unknown>={}) {
 if(!context||typeof window==='undefined')return;
 // Bound client memory and payloads too; never forward arbitrary Error objects.
 const safe:Record<string,unknown>={};for(const [key,value] of Object.entries(attributes))if(['string','number','boolean'].includes(typeof value))safe[key]=typeof value==='string'?value.slice(0,1024):value;
 if(queued.length<100)queued.push({...context,event,attributes:safe});
 if(!timer)timer=setTimeout(flush,250);
}
if(typeof window!=='undefined')window.addEventListener('pagehide',flush);
