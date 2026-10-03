// Real fMP4 decoding: no media clock, buffering, play(), or codec mocks.
import { build } from 'esbuild';
import { chromium, firefox, webkit } from 'playwright';
import { createServer, request as httpRequest } from 'node:http';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { tmpdir } from 'node:os';
import assert from 'node:assert/strict';
const root=resolve(import.meta.dirname,'..'),temporary=await mkdtemp(resolve(tmpdir(),'gazes-hls-browser-'));
const backend=process.env.PLAYBACK_TEST_URL||'http://172.22.0.3:8099';
const fixtures=await (await fetch(backend+'/fixtures')).json();
await build({absWorkingDir:root,stdin:{contents:`import React,{useEffect,useRef,useState} from 'react';import{createRoot}from'react-dom/client';import{HlsPlaybackController}from'./src/lib/hls-playback';import{SubtitleRenderer}from'./src/components/SubtitleRenderer';
function App(){const video=useRef(null),control=useRef(null);const[metadata,setMetadata]=useState(null);const[origin,setOrigin]=useState(0);const fixture=window.fixture;useEffect(()=>{const controller=new HlsPlaybackController(video.current,{state:s=>{window.snapshot=s;document.body.dataset.phase=s.phase;window.states.push(s);},metadata:(m,o)=>{setMetadata(m);setOrigin(o);},error:e=>window.failures.push(e),gesture:()=>window.gestures++});control.current=controller;window.seek=t=>controller.seek(t);window.pause=()=>controller.pause();window.play=()=>controller.play();window.audio=(track)=>controller.open(fixture.info_hash,0,track,video.current.currentTime);window.switchSource=(source)=>controller.open(source.info_hash,0,0,video.current.currentTime);window.dispose=()=>controller.dispose();controller.open(fixture.info_hash,0,0,Number(new URLSearchParams(location.search).get('start')||0));return()=>controller.dispose();},[]);const track=metadata?.subtitle_tracks?.[0];return <div style={{position:'relative',width:640,height:360}}><video ref={video} muted playsInline style={{width:'100%',height:'100%'}}/>{track&&<SubtitleRenderer videoRef={video} streamKey={fixture.info_hash} url={'/api/v1/subtitles?ih='+fixture.info_hash+'&file_idx=0&track_idx=0&format='+(track.codec==='hdmv_pgs_subtitle'?'sup':'ass')+'&timeline_origin='+origin} bitmap={track.codec==='hdmv_pgs_subtitle'} timeOffset={0} onError={(error)=>{window.subtitleError=error;}}/>}</div>};window.states=[];window.failures=[];window.gestures=0;createRoot(document.getElementById('root')).render(<App/>);`,loader:'tsx',resolveDir:root},outfile:resolve(temporary,'app.js'),bundle:true,format:'esm',platform:'browser',alias:{'@':resolve(root,'src')},define:{'process.env.NODE_ENV':'"production"','process.env.NEXT_PUBLIC_API_BASE':'""'}});
let requests=[];
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://localhost');
 if(url.pathname==='/'){const fixture=fixtures.find(f=>f.name===(url.searchParams.get('case')||'mkv'));res.setHeader('Content-Type','text/html');res.end(`<body style="background:black"><div id="root"></div><script>window.fixture=${JSON.stringify(fixture)}</script><script type="module" src="/app.js"></script>`);return;}
 if(url.pathname==='/app.js'){res.setHeader('Content-Type','text/javascript');res.end(await readFile(resolve(temporary,'app.js')));return;}
 if(url.pathname.startsWith('/subtitles/')){res.setHeader('Content-Type',url.pathname.endsWith('.js')?'text/javascript':url.pathname.endsWith('.wasm')?'application/wasm':'application/octet-stream');res.end(await readFile(resolve(root,'public',url.pathname.slice(1))));return;}
 requests.push({method:req.method,url:req.url,at:Date.now()});const proxy=httpRequest(new URL(req.url,backend),{method:req.method,headers:req.headers},upstream=>{res.writeHead(upstream.statusCode,upstream.headers);upstream.pipe(res);});proxy.on('error',()=>{if(!res.headersSent)res.writeHead(502);res.end();});res.on('close',()=>proxy.destroy());req.pipe(proxy);
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const base=`http://127.0.0.1:${server.address().port}`;
let browser;
try {
 const name=process.env.PLAYWRIGHT_BROWSER||'chromium';
 browser=await({chromium,firefox,webkit}[name]).launch({headless:true,...(name==='chromium'?{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/tmp/gazes-playwright/chromium-1243/chrome-linux64/chrome',args:['--no-sandbox','--autoplay-policy=no-user-gesture-required']}: {})});
 for(const kind of ['mkv','shifted','mp4']) {
  const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));page.setDefaultTimeout(30000);
  page.on('console',m=>{if(m.type()==='error'||process.env.DEBUG_HLS)console.error('browser',m.text());});
  await page.goto(base+`/?case=${kind}&start=8.35${process.env.DEBUG_HLS?'&debugHls=1':''}`);
  await page.waitForFunction(()=>document.querySelector('video')?.currentTime>8.35&&document.querySelector('video')?.videoWidth>0).catch(async error=>{console.error('State',await page.evaluate(()=>({snapshot:window.snapshot,failures:window.failures,states:window.states.slice(-5),video:{time:document.querySelector('video')?.currentTime,ready:document.querySelector('video')?.readyState,src:document.querySelector('video')?.src,error:document.querySelector('video')?.error?.message}})),requests.slice(-12));throw error;});
  await page.evaluate(()=>window.pause());
  await page.waitForFunction(()=>document.body.dataset.phase==='paused');
  await page.locator('video').evaluate(v=>window.originalVideo=v);
  await page.waitForFunction(()=>document.querySelector('video').buffered.length&&document.querySelector('video').buffered.end(0)>16);
  const before=requests.length;
  const cached=await page.evaluate(async()=>{const start=performance.now();await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(new Error('cached seek timed out')),5000);document.querySelector('video').addEventListener('seeked',()=>{clearTimeout(timer);resolve();},{once:true});window.seek(10.35);});return performance.now()-start;});
  assert.ok(cached<250,`${kind}: cached seek took ${cached}ms`);
  await page.waitForFunction(()=>document.body.dataset.phase==='paused');
  assert.equal(await page.locator('video').evaluate(v=>v===window.originalVideo),true);
  assert.equal(await page.locator('video').evaluate(v=>v.paused),true);
  assert.equal(requests.slice(before).filter(r=>r.url.includes('/2/media.m4s')).length,0,'cached seek performs no media request for the buffered destination');
  // The coded binary clock at the top of the actual decoded video must agree
  // with currentTime, including B-frames and nonzero source timestamps.
  async function clock(){return page.locator('video').evaluate(async v=>{const canvas=document.createElement('canvas');canvas.width=160;canvas.height=90;const ctx=canvas.getContext('2d');ctx.drawImage(v,0,0,160,90);let n=0;for(let bit=0;bit<7;bit++){if(ctx.getImageData(bit*20+10,10,1,1).data[0]>180)n|=1<<bit;}return {coded:n,time:v.currentTime};});}
  function assertFrame(frame,label){const expected=Math.floor(frame.time*25)%128;const distance=Math.min((frame.coded-expected+128)%128,(expected-frame.coded+128)%128);assert.ok(distance<=1,`${kind}: ${label} ${JSON.stringify(frame)} expected frame ${expected}`);}
  let frame=await clock();assertFrame(frame,'buffered frame');
  const distant=await page.evaluate(async()=>{const start=performance.now();await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(new Error('cold seek timed out')),5000);document.querySelector('video').addEventListener('seeked',()=>{clearTimeout(timer);resolve();},{once:true});window.seek(60.35);});return performance.now()-start;}).catch(async error=>{console.error('Cold failure',await page.evaluate(()=>({snapshot:window.snapshot,states:window.states.slice(-12),failures:window.failures,time:document.querySelector('video').currentTime,ready:document.querySelector('video').readyState})),requests.slice(-15));throw error;});
  await page.waitForFunction(()=>document.body.dataset.phase==='paused').catch(async error=>{console.error('Cold seek state',await page.evaluate(()=>({snapshot:window.snapshot,states:window.states.slice(-12),failures:window.failures,time:document.querySelector('video').currentTime,ready:document.querySelector('video').readyState,buffered:Array.from({length:document.querySelector('video').buffered.length},(_,n)=>[document.querySelector('video').buffered.start(n),document.querySelector('video').buffered.end(n)])})));throw error;});
  assert.ok(distant<2000,`${kind}: cold seek took ${distant}ms`);
  frame=await clock();assertFrame(frame,'cold frame');
  await page.evaluate(()=>{window.seek(20.35);window.seek(44.35);window.seek(12.35);});
  await page.waitForFunction(()=>document.body.dataset.phase==='paused'&&Math.abs(document.querySelector('video').currentTime-12.35)<0.04).catch(async error=>{console.error('Rapid seek state',await page.evaluate(()=>({snapshot:window.snapshot,states:window.states.slice(-12),failures:window.failures,time:document.querySelector('video').currentTime,ready:document.querySelector('video').readyState})));throw error;});
  frame=await clock();assertFrame(frame,'last seek frame');
  await page.evaluate(()=>window.audio(1));
  await page.waitForFunction(()=>document.body.dataset.phase==='paused'&&Math.abs(document.querySelector('video').currentTime-12.35)<0.04);
  frame=await clock();assertFrame(frame,'audio switch frame');
  assert.equal(await page.locator('video').evaluate(v=>v===window.originalVideo),true,'audio switch preserves the video');
  await page.evaluate(source=>window.switchSource(source),fixtures.find(f=>f.name==='shifted'));
  await page.waitForFunction(()=>document.body.dataset.phase==='paused'&&Math.abs(document.querySelector('video').currentTime-12.35)<0.04);
  assert.equal(await page.locator('video').evaluate(v=>v===window.originalVideo),true,'source switch preserves the video');
  assert.deepEqual(await page.evaluate(()=>window.failures),[]);assert.deepEqual(errors,[]);
  console.log(`PASS ${name}/${kind}: real decoding, cached ${cached.toFixed(0)}ms / cold ${distant.toFixed(0)}ms, coded clock, pause, rapid seeks, audio/source switch`);
  await page.evaluate(()=>window.dispose());await page.close();
 }
} finally {await browser?.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(temporary,{recursive:true,force:true});}
