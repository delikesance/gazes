import { build } from 'esbuild';
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import assert from 'node:assert/strict';
const root=resolve(import.meta.dirname,'..'), temp=await mkdtemp(resolve(tmpdir(),'gazes-vf-player-'));
const source={id:'vf',info_hash:'vf',magnet_uri:'magnet:?xt=urn:btih:vf',title:'Naruto VF',episode_number:1,season_number:1,anime_aliases:['Naruto'],language_tag:'VF'};
const english={...source,id:'english',info_hash:'english',magnet_uri:'magnet:?xt=urn:btih:english',title:'Naruto Dual Audio',language_tag:'OTHER',seeders:60,score_rank:300};
const multi={...source,language_tag:'MULTI',seeders:20,score_rank:200};
const entry=`import React from 'react';import {createRoot} from 'react-dom/client';import {VideoPlayerModal} from './src/components/VideoPlayerModal';import {AutoEpisodePlayer as AEP} from './src/components/AutoEpisodePlayer';import {AuthProvider} from './src/components/AuthProvider';const AutoEpisodePlayer=p=><AuthProvider><AEP {...p}/></AuthProvider>;const ready=new URLSearchParams(location.search).has('ready');createRoot(document.getElementById('root')).render(location.search ? <AutoEpisodePlayer sources={ready?[${JSON.stringify(english)}]:[{...${JSON.stringify(source)},info_hash:'offline',magnet_uri:'magnet:?xt=urn:btih:offline'}]} animeId={20} seasonId={20} partial={true} animeTitle="Naruto" episodeNumber={1} onClose={()=>{}}/> : <VideoPlayerModal item={${JSON.stringify(source)}} onClose={()=>{}}/>);`;
await build({absWorkingDir:root,stdin:{contents:entry,loader:'tsx',resolveDir:root},outfile:resolve(temp,'app.js'),bundle:true,format:'esm',platform:'browser',alias:{'@':resolve(root,'src')},define:{'process.env.NODE_ENV':'"production"','process.env.NEXT_PUBLIC_API_BASE':'"/api/v1"'}});
const streams=[],probed=[],loaded=[];let fullDiscoveries=0,scenario='basic';
const meta={probe_status:'complete',duration_sec:120,video_codec:'h264',audio_tracks:[{index:0,language:'jpn',title:'Japanese',codec:'aac',channels:2,is_default:true},{index:1,language:'fre',title:'French',codec:'aac',channels:2,is_default:false}],subtitle_tracks:[]};
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://localhost');
 if(url.pathname==='/'){res.setHeader('Content-Type','text/html');res.end('<div id="root"></div><script type="module" src="/app.js"></script>');return;}
 if(url.pathname==='/app.js'){res.setHeader('Content-Type','text/javascript');res.end(await readFile(resolve(temp,'app.js')));return;}
 if(url.pathname.endsWith('/sources')){assert.equal(url.searchParams.get('discovery'),'full');fullDiscoveries++;res.setHeader('Content-Type','application/json');res.end(JSON.stringify({sources:scenario==='basic'?[source]:scenario==='fallback'?[english]:[english,multi],partial:false}));return;}
 if(url.pathname.endsWith('/torrent/load')){let body='';for await(const chunk of req)body+=chunk;const hash=JSON.parse(body).magnet.split(':').pop();if(!req.headers['x-gazes-prewarm'])loaded.push(hash);if(hash==='offline'){res.statusCode=504;res.end('offline');return;}res.setHeader('Content-Type','application/json');res.end(JSON.stringify({info_hash:hash,files:[{index:0,path:'Naruto 001.mkv',is_video:true},{index:1,path:'Naruto 002.mkv',is_video:true}],main_video_index:1,main_video_metadata:{...meta,audio_tracks:[]}}));return;}
 if(url.pathname.endsWith('/metadata')){probed.push(url.searchParams.get('file_idx'));await new Promise(resolve=>setTimeout(resolve,200));res.setHeader('Content-Type','application/json');res.end(JSON.stringify(url.searchParams.get('ih')==='english'?{...meta,audio_tracks:scenario==='unknown'?[{index:0,language:'und',title:'Audio',codec:'aac'}]:[{index:0,language:'eng',title:'English',codec:'aac'},{index:1,language:'jpn',title:'Japanese',codec:'aac'}]}:meta));return;}
 if(url.pathname.endsWith('/stream')){streams.push(url);res.setHeader('Content-Type','video/mp4');res.flushHeaders();return;}
 res.setHeader('Content-Type','application/json');res.end('{}');
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']});
 const page=await browser.newPage({locale:'fr-FR'}),errors=[];page.on('pageerror',error=>errors.push(error.message));
 await page.addInitScript(()=>{
  HTMLMediaElement.prototype.play=function(){return Promise.resolve();};HTMLMediaElement.prototype.pause=function(){};
  Object.defineProperty(HTMLVideoElement.prototype,'videoWidth',{get(){return 1280;}});
  Object.defineProperty(HTMLMediaElement.prototype,'currentTime',{get(){return this.clock||0;},set(value){this.clock=value;}});
 });
 await page.goto(`http://127.0.0.1:${server.address().port}`);
 await page.waitForFunction(()=>{const video=document.querySelector('video');return video&&new URL(video.src).searchParams.get('audio_track')==='1';},null,{timeout:10000}).catch(async error=>{console.error({errors,probed,streams:streams.map(String),body:await page.locator('body').innerText()});throw error;});
 assert.deepEqual(probed,['0'],'probe the selected episode rather than the largest file');
 assert.ok(streams.some(url=>url.searchParams.get('file_idx')==='0'&&url.searchParams.get('audio_track')==='1'));
 await page.getByTitle(/piste audio|Select Audio Track/i).dispatchEvent('click');
 await page.getByRole('button',{name:/Japonais/}).dispatchEvent('click');
 await page.waitForFunction(()=>(new URL(document.querySelector('video').src).searchParams.get('audio_track')||'0')==='0');
 await page.waitForTimeout(300);
 assert.equal(new URL(await page.locator('video').getAttribute('src'),'http://localhost').searchParams.get('audio_track')||'0','0','do not overwrite a manual choice after metadata arrived');
 await page.goto(`http://127.0.0.1:${server.address().port}/?full=1`);
 await page.waitForFunction(()=>{const video=document.querySelector('video');return video&&new URL(video.src).searchParams.get('ih')==='vf'&&new URL(video.src).searchParams.get('audio_track')==='1';},null,{timeout:10000});
 assert.equal(fullDiscoveries,1,'a failed initial source triggers full discovery exactly once and resumes with its VF fallback');
 scenario='available-vf';loaded.length=0;probed.length=0;
 await page.goto(`http://127.0.0.1:${server.address().port}/?full=1`);
 await page.getByTitle(/piste audio|Select Audio Track/i).waitFor();
 await page.waitForTimeout(700);
 assert.deepEqual(loaded,['offline','english'],'missing VF keeps the first playable source even when a French fallback is available');
 assert.equal(new URL(await page.locator('video').getAttribute('src'),'http://localhost').searchParams.get('ih'),'english');
 assert.deepEqual(probed,['0'],'no probe of another source merely to find VF');
 await page.locator('video').evaluate(video=>{video.dispatchEvent(new Event('playing'));video.currentTime=7;video.dispatchEvent(new Event('timeupdate'));});
 await page.getByText('Cette source ne répond pas.',{exact:true}).waitFor({state:'hidden'});
 assert.equal(await page.getByText('Cette source ne répond pas.',{exact:true}).count(),0,'missing VF is not a source failure');
 // An actual video error still advances to the French fallback and selects its VF.
 await page.locator('video').evaluate(video=>video.dispatchEvent(new Event('error')));
 await page.waitForFunction(()=>{const video=document.querySelector('video');return video&&new URL(video.src).searchParams.get('ih')==='vf'&&new URL(video.src).searchParams.get('audio_track')==='1';},null,{timeout:10000});
 assert.deepEqual(loaded,['offline','english','vf']);
 assert.equal(new URL(await page.locator('video').getAttribute('src'),'http://localhost').searchParams.get('time_offset'),'7','real source failure preserves playback position');
 scenario='fallback';loaded.length=0;
 await page.goto(`http://127.0.0.1:${server.address().port}/?full=1`);
 await page.waitForFunction(()=>document.querySelector('video')?.src.includes('ih=english'),null,{timeout:10000});
 await page.waitForTimeout(700);
 assert.equal(loaded.filter(id=>id==='english').length,1,'a non-French source must never be deferred and loaded again');
 for(const audio of ['absent','unknown']){
  scenario=audio;loaded.length=0;probed.length=0;const discoveriesBefore=fullDiscoveries;
  await page.goto(`http://127.0.0.1:${server.address().port}/?ready=1`);
  await page.getByTitle(/piste audio|Select Audio Track/i).waitFor();
  await page.waitForTimeout(700);
  assert.deepEqual(loaded,['english'],`${audio} VF must keep the initial source`);
  assert.deepEqual(probed,['0']);
  assert.equal(fullDiscoveries,discoveriesBefore,`${audio} VF must not trigger full discovery`);
  assert.equal(new URL(await page.locator('video').getAttribute('src'),'http://localhost').searchParams.get('ih'),'english');
  assert.equal(await page.getByText('Cette source ne répond pas.',{exact:true}).count(),0);
 }
 assert.deepEqual(errors,[]);
 console.log('PASS: correct episode probe, French audio preferred, manual choice preserved, actual failures still switch sources, absent/unknown VF plays immediately without full discovery or reloading.');
}finally{await browser?.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(temp,{recursive:true,force:true});}
