// Real automatic player; simulated video events test source policy and timers.
import { build } from 'esbuild';
import { firefox, chromium } from 'playwright';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import assert from 'node:assert/strict';
const root=resolve(import.meta.dirname,'..'), temp=await mkdtemp(resolve(tmpdir(),'gazes-auto-'));
const ids=['ambiguous','offline','metadata-stalled','stalled','good','last'];
const sources=ids.map((id,index)=>({id,info_hash:id,magnet_uri:`magnet:?xt=urn:btih:${id}`,title:id,quality:'1080p',release_group:'Test',language_tag:id==='last'?'VF':'VOSTFR',is_french:true,score_rank:100-index*10,seeders:5,is_batch:true,episode_number:14,season_number:1,size_bytes:1000,anime_aliases:['Example']}));
// Reverse input and include a duplicate: attempts must still follow ranking.
const entry=`import React from 'react'; import {createRoot} from 'react-dom/client'; import {AutoEpisodePlayer} from './src/components/AutoEpisodePlayer'; import {PLAYBACK_TIMEOUTS} from './src/lib/playback-sources'; Object.assign(PLAYBACK_TIMEOUTS,{metadata:1000,startup:2500,stall:1000}); const root=createRoot(document.getElementById('root')); root.render(<AutoEpisodePlayer sources={${JSON.stringify([...sources].reverse().concat(sources[4]))}} animeTitle="Example" episodeNumber={14} onClose={()=>{document.body.dataset.closed='true';root.unmount();}}/>);`;
await build({absWorkingDir:root,stdin:{contents:entry,loader:'tsx',resolveDir:root},outfile:resolve(temp,'app.js'),bundle:true,format:'esm',platform:'browser',alias:{'@':resolve(root,'src')},define:{'process.env.NODE_ENV':'"production"','process.env.NEXT_PUBLIC_API_BASE':'"/api/v1"'}});
const attempts=[], streamRequests=[], diagnosticEvents=[];
let metadataAborted=false;
const metadata={duration_sec:120,video_codec:'hevc',audio_tracks:[{index:0,title:'Japanese',language:'jpn'}],subtitle_tracks:[]};
const files=Array.from({length:500},(_,index)=>({index,path:`Example - ${String(index+1).padStart(3,'0')}.mkv`,length:1000,is_video:true}));
let failedLast=false;
const server=createServer(async (req,res)=>{
 try {
 const url=new URL(req.url,'http://localhost');
 if(url.pathname==='/') {res.setHeader('Content-Type','text/html');res.end('<div id="root"></div><script type="module" src="/app.js"></script>');return;}
 if(url.pathname==='/app.js'){res.setHeader('Content-Type','text/javascript');res.end(await readFile(resolve(temp,'app.js')));return;}
 if(url.pathname.endsWith('/diagnostics/events')) {
  let body='';for await(const chunk of req)body+=chunk;
  diagnosticEvents.push(...JSON.parse(body).events);res.statusCode=204;res.end();return;
 }
 if(url.pathname.endsWith('/torrent/load')) {
  let body='';for await (const chunk of req) body+=chunk;
  const payload=JSON.parse(body), id=payload.magnet.split(':').pop();
  assert.equal(payload.metadata_only,true);
  if(req.headers['x-gazes-prewarm']){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({info_hash:id,files,main_video_index:0,main_video_metadata:metadata}));return;}
  attempts.push(id);
  if(id==='metadata-stalled'){res.setHeader('Content-Type','application/json');res.write('{');res.on('close',()=>metadataAborted=true);return;}
  if(id==='offline'||(id==='last'&&failedLast)){res.statusCode=504;res.end('unavailable');return;}
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify({info_hash:id,files:id==='ambiguous'?[{index:0,path:'unknown.mkv',is_video:true}]:files,main_video_index:0,main_video_metadata:metadata}));return;
 }
 if(url.pathname.endsWith('/metadata')) {assert.equal(url.searchParams.get('file_idx'),'13');res.setHeader('Content-Type','application/json');res.end(JSON.stringify(metadata));return;}
 if(url.pathname.endsWith('/stream')) {streamRequests.push(url);assert.equal(url.searchParams.get('file_idx'),'13');res.setHeader('Content-Type','video/mp4');res.flushHeaders();res.on('close',()=>res.end());return;}
 res.setHeader('Content-Type','application/json');res.end('{}');
 } catch(error){res.statusCode=500;res.end(String(error));}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try {
 browser=process.env.PLAYWRIGHT_BROWSER==='chromium' ? await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium',args:['--no-sandbox','--enable-unsafe-swiftshader']}) : await firefox.launch({headless:true,...(process.env.FIREFOX_PATH?{executablePath:process.env.FIREFOX_PATH}:{})});
 const page=await browser.newPage({locale:'fr-FR'});page.setDefaultTimeout(10000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(()=>{
  HTMLMediaElement.prototype.canPlayType=()=> 'probably';
  Object.defineProperty(HTMLVideoElement.prototype,'videoWidth',{get(){return 1280;}});
  Object.defineProperty(HTMLMediaElement.prototype,'currentTime',{get(){return this.clock||0;},set(value){this.clock=value;}});
  Object.defineProperty(HTMLMediaElement.prototype,'paused',{get(){return !this.running;}});
  HTMLMediaElement.prototype.play=function(){this.running=true;this.dispatchEvent(new Event('play'));return Promise.resolve();};
  HTMLMediaElement.prototype.pause=function(){this.running=false;this.dispatchEvent(new Event('pause'));};
 });
 await page.goto(`http://127.0.0.1:${server.address().port}`);
 await page.waitForFunction(()=>document.querySelector('video')?.src.includes('ih=good'),null,{polling:50}).catch(async error=>{console.error('Attempts at timeout:',attempts,'Browser errors:',errors,'Player:',await page.locator('body').innerText());throw error;});
 console.log('Initial attempts:',attempts);
 assert.deepEqual(attempts,ids.slice(0,5));
 assert.equal(metadataAborted,true,'abandoning metadata must cancel the old request');
 assert.equal(streamRequests.some(url=>url.searchParams.get('ih')==='ambiguous'),false,'do not download an arbitrary pack file');
 assert.equal(await page.locator('select').count(),0,'automatic mode must not ask to choose a file or source');
 await page.locator('video').evaluate(video=>{video.dispatchEvent(new Event('canplay'));video.currentTime=7;video.dispatchEvent(new Event('timeupdate'));});
 // Browser policy rejection needs a gesture, not a different swarm.
 await page.locator('video').evaluate(video=>{
  window.originalPlay=HTMLMediaElement.prototype.play;
  HTMLMediaElement.prototype.play=()=>Promise.reject(new DOMException('Autoplay blocked','NotAllowedError'));
  video.running=false;video.dispatchEvent(new Event('canplay'));
 });
 await page.getByRole('button',{name:'Lecture',exact:true}).waitFor();
 await page.waitForTimeout(2800);
 assert.equal(attempts.at(-1),'good','blocked autoplay must keep the source beyond startup/stall deadlines');
 await page.evaluate(()=>HTMLMediaElement.prototype.play=window.originalPlay);
 await page.getByRole('button',{name:'Lecture',exact:true}).click();
 await page.getByTitle('Pause (Espace)',{exact:true}).click();
 await page.waitForTimeout(1600);
 assert.equal(attempts.at(-1),'good','paused playback must not trigger source fallback');
 await page.getByTitle('Lecture (Espace)',{exact:true}).click();
 // No new timeupdate: a stalled playing source must advance to the next one.
 await page.waitForFunction(()=>document.querySelector('video')?.src.includes('ih=last'),null,{polling:50});
 console.log('Stall fallback:',attempts);
 assert.equal(new URL(await page.locator('video').getAttribute('src'),'http://localhost').searchParams.get('time_offset'),'7','fallback must preserve playback position');
 await page.locator('video').evaluate(video=>video.dispatchEvent(new Event('error')));
 await page.getByText('Toutes les tentatives de lecture ont échoué.').waitFor();
 assert.deepEqual(attempts,ids,'never repeat a failed source or duplicate magnet');
 failedLast=true;
 await page.getByRole('button',{name:'Réessayer',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('video')?.src.includes('ih=stalled'),null,{polling:50});
 await page.getByRole('button',{name:'Fermer le lecteur',exact:true}).click();
 await page.waitForFunction(()=>document.body.dataset.closed==='true',null,{polling:50});
 await page.waitForTimeout(500);
 assert.ok(diagnosticEvents.some(e=>e.event==='playback.file_rejected'),'ambiguous pack must have a recorded reason');
 assert.ok(diagnosticEvents.some(e=>e.event==='playback.file_selected'&&e.attributes.file_index===13),'selected episode must be recorded');
 assert.ok(diagnosticEvents.some(e=>e.event==='playback.started'),'actual video advancement must be recorded');
 assert.ok(diagnosticEvents.filter(e=>e.event==='playback.failed').length>=5,'source failures must be recorded');
 assert.equal(new Set(diagnosticEvents.map(e=>e.playback_session_id)).size,1,'all attempts must share one session');
 assert.ok(new Set(diagnosticEvents.filter(e=>e.event==='playback.attempt').map(e=>e.attempt_id)).size>=6,'attempts must have distinct IDs');
 // Manual source selection uses the existing player and preserves absolute time.
 failedLast=false;
 await page.goto(`http://127.0.0.1:${server.address().port}`);
 await page.waitForFunction(()=>document.querySelector('video')?.src.includes('ih=good'));
 await page.locator('video').evaluate(video=>{video.dispatchEvent(new Event('canplay'));video.currentTime=23;video.dispatchEvent(new Event('timeupdate'));video.pause();});
 await page.getByRole('button',{name:'Changer de source pour cet épisode',exact:true}).click();
 const dialog=page.getByRole('dialog');
 await dialog.waitFor();
 assert.equal(await dialog.getByRole('button',{name:'Source actuelle',exact:true}).isDisabled(),true);
 await page.keyboard.press('Escape');
 await dialog.waitFor({state:'hidden'});
 assert.ok((await page.locator('video').getAttribute('src')).includes('ih=good'),'closing source picker keeps playback');
 await page.getByRole('button',{name:'Changer de source pour cet épisode',exact:true}).click();
 await dialog.getByRole('button',{name:'VF',exact:true}).click();
 assert.equal(await dialog.locator('.group').count(),1,'VF filter isolates French dubbed releases');
 await dialog.locator('.group').filter({has:page.getByText('last',{exact:true})}).getByRole('button').click();
 await page.waitForFunction(()=>document.querySelector('video')?.src.includes('ih=last'));
 assert.equal(new URL(await page.locator('video').getAttribute('src'),'http://localhost').searchParams.get('time_offset'),'23','manual switch resumes at the current position');
 assert.equal(await dialog.count(),0);
 await page.locator('video').evaluate(video=>{video.dispatchEvent(new Event('canplay'));video.pause();});
 await page.getByTitle('Plein écran (F)',{exact:true}).click();
 await page.waitForFunction(()=>Boolean(document.fullscreenElement));
 await page.getByRole('button',{name:'Changer de source pour cet épisode',exact:true}).click();
 assert.equal(await dialog.evaluate(element=>document.fullscreenElement.contains(element)),true,'source selector remains visible in fullscreen');
 await page.keyboard.press('Escape');
 await dialog.waitFor({state:'hidden'});
 assert.deepEqual(errors,[]);
 console.log('PASS: score ranking, pack matching, metadata-only load, errors/startup/stall fallback, paused state, resume position, deduplication, exhaustion, retry, manual source selection, VF filter, resume position and fullscreen.');
}finally{await browser?.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(temp,{recursive:true,force:true});}
