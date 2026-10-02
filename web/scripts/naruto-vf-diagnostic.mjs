// Live metadata audit: no playback and no complete video downloads.
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { episodeFile } from '../src/lib/episode-file.ts';
import { vfStatus } from '../src/lib/media-tracks.ts';

const base = process.env.DIAGNOSTIC_API_BASE || 'http://127.0.0.1:8080/api/v1';
const output = resolve(process.env.DIAGNOSTIC_OUTPUT || new URL('../../docs/naruto-vf-audit', import.meta.url).pathname);
const episodes = [1, 5, 10, 15, 20];
const sourceBudget = 90_000, idleBudget = 30_000, episodeBudget = 300_000;
const report = { started: new Date().toISOString(), base, episodes: [], success: false };
let contextHeaders = {};
await mkdir(output, { recursive: true });
const persist = () => writeFile(resolve(output, 'report.json'), JSON.stringify(report, null, 2) + '\n');
async function request(path, options = {}, budget = 10_000) {
 const response = await fetch(base + path, { ...options, headers:{...contextHeaders,...options.headers}, signal: AbortSignal.timeout(Math.max(1, budget)) });
 if (!response.ok) throw new Error(`HTTP ${response.status}: ${await response.text()}`);
 return response.json();
}
const languageOrder = source => source.language_tag === 'VF' ? 0 : source.language_tag === 'VOSTFR' ? 1 : 2;
const rank = sources => [...sources].sort((a,b) => languageOrder(a)-languageOrder(b) || Number(b.seeders>0)-Number(a.seeders>0) || Number(b.is_batch)-Number(a.is_batch) || (b.score_breakdown?.quality||0)-(a.score_breakdown?.quality||0) || b.score_rank-a.score_rank);
for (const episode of episodes) {
 const started = Date.now(), deadline = started + episodeBudget;
 const result = { episode, started: new Date(started).toISOString(), status: 'inaccessible', attempts: [], searches: [] };
 result.trace_id = randomUUID();
 contextHeaders = {'X-Playback-Session-ID':result.trace_id,'X-Playback-Anime-ID':'20','X-Playback-Season-ID':'20','X-Playback-Episode':String(episode)};
 report.episodes.push(result);
 const tried = new Set();
 for (const discovery of ['fast', 'full']) {
  if (result.status === 'confirmed' || Date.now() >= deadline) break;
  let sources;
  try {
   const t = Date.now();
   const data = await request(`/catalog/anime/20/seasons/20/episodes/${episode}/sources?discovery=${discovery}`, {}, Math.min(40_000, deadline-Date.now()));
   result.searches.push({ discovery, elapsed_ms: Date.now()-t, partial: data.partial, sources: data.total_sources });
   const capture = {...data, sources:(data.sources || []).map(({anime_aliases,excluded_titles,...source})=>source)};
   capture.identity = {anime_aliases:data.sources?.[0]?.anime_aliases,excluded_titles:data.sources?.[0]?.excluded_titles};
   await writeFile(resolve(output, `episode-${episode}-${discovery}-sources.json`), JSON.stringify(capture, null, 2)+'\n');
   sources = rank(data.sources || []).filter(source=>discovery==='full' || source.language_tag==='VF');
  } catch(error) { result.searches.push({discovery, error: String(error)}); await persist(); continue; }
  for (const source of sources) {
   if (Date.now() >= deadline || result.status === 'confirmed') break;
   if (tried.has(source.info_hash)) continue;
   tried.add(source.info_hash);
   const t = Date.now(), sourceDeadline = Math.min(t+sourceBudget, deadline);
   const attempt = { title: source.title, infohash: source.info_hash, score: source.score_rank, seeders: source.seeders,
    language_claim: source.language_tag, status: 'inaccessible', probes: [], swarm: [] };
   result.attempts.push(attempt);
   attempt.attempt_id = randomUUID();
   contextHeaders['X-Playback-Attempt-ID'] = attempt.attempt_id;
   console.log(JSON.stringify({episode, event:'attempt', title:source.title, discovery}));
   let lastActivity = Date.now(), lastBytes = null;
   try {
    const loaded = await request('/torrent/load', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({magnet:source.magnet_uri, metadata_only:true})}, Math.min(idleBudget, sourceDeadline-Date.now()));
    attempt.metadata_elapsed_ms = Date.now()-t;
    const index = episodeFile(loaded.files, source);
    if (index === null) throw new Error('episode_missing_or_ambiguous');
    attempt.file = loaded.files.find(file=>file.index===index);
    while (Date.now() < sourceDeadline && Date.now()-lastActivity < idleBudget) {
     const probeStart = Date.now();
     let meta;
     try {
      meta = await request(`/metadata?ih=${source.info_hash}&file_idx=${index}`, {}, Math.min(7_000, sourceDeadline-Date.now()));
      attempt.probes.push({elapsed_ms:Date.now()-probeStart, metadata:meta});
      if (meta.probe_status === 'complete') {
       attempt.status = vfStatus(meta);
       attempt.metadata = meta;
       break;
      }
     } catch(error) { attempt.probes.push({elapsed_ms:Date.now()-probeStart, error:String(error)}); }
     try {
      const stats = await request(`/torrent/stats?ih=${source.info_hash}`, {}, Math.min(2_000, sourceDeadline-Date.now()));
      attempt.swarm.push({elapsed_ms:Date.now()-t, ...stats});
      if (lastBytes !== null && stats.completed_bytes > lastBytes) lastActivity = Date.now();
      lastBytes = stats.completed_bytes;
     } catch { /* Probe status is the evidence; telemetry is best effort. */ }
     await delay(Math.max(0, Math.min(1000, sourceDeadline-Date.now(), idleBudget-(Date.now()-lastActivity))));
    }
    if (attempt.status === 'inaccessible') attempt.reason = Date.now()-lastActivity >= idleBudget ? 'swarm_idle' : 'source_budget_exhausted';
   } catch(error) { attempt.reason = String(error); }
   attempt.elapsed_ms = Date.now()-t;
   if (attempt.status === 'confirmed') { result.status='confirmed'; result.confirmed_source=source.info_hash; result.file=attempt.file; }
   else if (result.status !== 'confirmed' && attempt.status === 'unknown') result.status='unknown';
   else if (result.status === 'inaccessible' && attempt.status === 'absent') result.status='absent';
   await persist();
   console.log(JSON.stringify({episode, event:'result', status:attempt.status, reason:attempt.reason, elapsed_ms:attempt.elapsed_ms}));
  }
 }
 result.elapsed_ms = Date.now()-started;
 result.finished = new Date().toISOString();
 await persist();
}
report.finished = new Date().toISOString();
report.success = report.episodes.every(episode=>episode.status==='confirmed');
await persist();
const lines = ['# Vérification VF Naruto', '', '| Épisode | Résultat | Fichier | Durée des essais |', '| ---: | --- | --- | ---: |'];
for(const episode of report.episodes) lines.push(`| ${episode.episode} | ${episode.status} | ${episode.file?.path || '—'} | ${(episode.elapsed_ms/1000).toFixed(1)} s |`);
lines.push('', 'Une réussite exige une piste audio française dans les métadonnées du bon épisode. Les timeouts ne prouvent pas son absence.', '', '[Captures détaillées](report.json)');
await writeFile(resolve(output, 'README.md'), lines.join('\n')+'\n');
console.log(JSON.stringify({success:report.success, episodes:report.episodes.map(e=>({episode:e.episode,status:e.status})), output}));
process.exitCode = report.success ? 0 : 1;
