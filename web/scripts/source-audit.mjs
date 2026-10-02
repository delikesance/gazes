// Verify actual audio tracks of the exact episode, without starting playback.
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { episodeFile } from '../src/lib/episode-file.ts';
import { vfStatus } from '../src/lib/media-tracks.ts';

const args = new Map();
for (let i=2;i<process.argv.length;i+=2) {
 const key=process.argv[i], value=process.argv[i+1];
 if (!['--url','--capture','--out','--limit','--source-ms'].includes(key) || !value) throw new Error(`Unknown or missing argument: ${key}`);
 args.set(key,value);
}
const page = new URL(args.get('--url') || 'http://127.0.0.1:8080/anime/101280/seasons/101280/episodes/1');
const match = page.pathname.match(/^\/anime\/(\d+)\/seasons\/(\d+)\/episodes\/(\d+)\/?$/);
if (!match) throw new Error('--url must be an episode page URL');
const [,anime,season,episode]=match;
const base = `${page.origin}/api/v1`;
const limit = Number(args.get('--limit') || 8), budget = Number(args.get('--source-ms') || 25_000);
if (!Number.isInteger(limit) || limit<1 || limit>50 || !Number.isFinite(budget) || budget<1000 || budget>120_000) throw new Error('Invalid --limit (1–50) or --source-ms (1000–120000)');
const output=resolve(args.get('--out') || 'docs/source-track-audit');
await mkdir(output,{recursive:true});
const headers={'X-Playback-Session-ID':randomUUID(),'X-Playback-Anime-ID':anime,'X-Playback-Season-ID':season,'X-Playback-Episode':episode};
const report={url:page.href,started:new Date().toISOString(),attempts:[],coverage:'bounded',vf_confirmed:false};
const save=()=>writeFile(resolve(output,'tracks.json'),JSON.stringify(report,null,2)+'\n');
async function request(path, options={}, timeout=10_000) {
 const response=await fetch(base+path,{...options,headers:{...headers,...options.headers},signal:AbortSignal.timeout(Math.max(1,timeout))});
 if (!response.ok) throw new Error(`HTTP ${response.status}: ${(await response.text()).slice(0,200)}`);
 return response.json();
}
const capture=args.has('--capture') ? JSON.parse(await readFile(args.get('--capture'),'utf8')) : await request(`/catalog/anime/${anime}/seasons/${season}/episodes/${episode}/sources?discovery=full`,{},40_000);
const response=capture.result || capture;
if (!Array.isArray(response.sources)) throw new Error('Capture contains no source response');
if (Number(response.episode_number)!==Number(episode)) throw new Error('Capture episode does not match --url');
const catalog=await request(`/catalog/anime/${anime}/seasons/${season}`);
const titleKey=value=>String(value||'').normalize('NFKD').replace(/\p{M}/gu,'').toLowerCase().replace(/[^\p{L}\p{N}]+/gu,'');
const targetTitles=new Set([...(catalog.aliases||[]),catalog.display_title,catalog.title_english,catalog.title_romaji].map(titleKey).filter(Boolean));
if(Number(catalog.id)!==Number(season) || !targetTitles.has(titleKey(response.anime_title))) throw new Error('Capture anime does not match the requested catalog season');
report.discovery_partial=Boolean(response.partial);
report.available_sources=response.sources.length;
const priority=s=>s.language_tag==='VF'?0:s.language_tag==='MULTI'?1:s.language_tag==='VOSTFR'?2:3;
const seen=new Set();
const sources=[...response.sources].sort((a,b)=>Number(b.seeders>0)-Number(a.seeders>0)||priority(a)-priority(b)||b.seeders-a.seeders).filter(s=>{if(!s.info_hash||seen.has(s.info_hash))return false;seen.add(s.info_hash);return true;}).slice(0,limit);
for (const source of sources) {
 const started=Date.now(), deadline=started+budget;
 const attempt={title:source.title,info_hash:source.info_hash,language_claim:source.language_tag,status:'inaccessible'};
 report.attempts.push(attempt);
 headers['X-Playback-Attempt-ID']=randomUUID();
 try {
  const loaded=await request('/torrent/load',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({magnet:source.magnet_uri,metadata_only:true})},Math.min(10_000,deadline-Date.now()));
	  attempt.files=loaded.files;
  const index=episodeFile(loaded.files,source);
  if(index===null) { attempt.status='episode_unverified'; attempt.reason='file missing or ambiguous'; }
  else {
   attempt.file=loaded.files.find(f=>f.index===index);
   while(Date.now()<deadline) {
    try {
     const meta=await request(`/metadata?ih=${source.info_hash}&file_idx=${index}`,{},Math.min(7000,deadline-Date.now()));
     attempt.metadata=meta;
     if(meta.probe_status==='complete') { attempt.status=vfStatus(meta); break; }
    } catch(error) {
     // One slow response must not consume the whole source inspection budget.
     attempt.last_probe_error=String(error);
    }
    await delay(Math.max(0,Math.min(1000,deadline-Date.now())));
   }
   if(attempt.status==='inaccessible') attempt.reason='probe deadline; does not prove VF absence';
  }
 } catch(error) { attempt.reason=String(error); }
 attempt.elapsed_ms=Date.now()-started;
 report.vf_confirmed ||= attempt.status==='confirmed';
 await save();
 console.log(JSON.stringify({title:attempt.title,status:attempt.status,elapsed_ms:attempt.elapsed_ms}));
}
report.finished=new Date().toISOString();
await save();
console.log(JSON.stringify({output,vf_confirmed:report.vf_confirmed,attempted:report.attempts.length,available:report.available_sources}));
// This is a diagnostic, not a claim that French audio exists in the swarm.
