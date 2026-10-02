// Render actual ASS/PGS chunks and exercise retries, seek cancellation and reuse.
import {build} from 'esbuild';
import {chromium} from 'playwright';
import {createServer} from 'node:http';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {resolve} from 'node:path';
import {spawnSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=resolve(import.meta.dirname,'..'),temp=await mkdtemp(resolve(tmpdir(),'gazes-subtitle-windows-'));
const sup=await readFile(resolve(root,'node_modules/libpgs/tests/files/test.sup'));
const fixture=shift=>{
 const data=Buffer.from(sup);let pos=0;
 while(pos<data.length){for(const off of [2,6])data.writeUInt32BE(data.readUInt32BE(pos+off)+shift*90000,pos+off);pos+=13+data.readUInt16BE(pos+11);}
 return data;
};
const stamp=n=>`${Math.floor(n/3600)}:${String(Math.floor(n/60)%60).padStart(2,'0')}:${String(n%60).padStart(2,'0')}.00`;
const ass=shift=>`[Script Info]\nScriptType: v4.00+\nPlayResX: 128\nPlayResY: 64\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Liberation Sans,12,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,0,2,2,2,2,1\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,${stamp(shift+1)},${stamp(shift+4)},Default,,0,0,0,,Window subtitles\n`;
await build({absWorkingDir:root,stdin:{contents:`import React,{useRef} from 'react';import {createRoot} from 'react-dom/client';import {SubtitleRenderer} from './src/components/SubtitleRenderer';const root=createRoot(document.getElementById('root'));window.unmount=()=>root.unmount();function App(){const ref=useRef(null);return <div style={{position:'relative',width:512,height:256}}><video ref={ref} style={{width:'100%',height:'100%'}}/><SubtitleRenderer videoRef={ref} url={'/api/subtitles?format='+ (location.search.includes('pgs')?'sup':'ass')} bitmap={location.search.includes('pgs')} timeOffset={location.search.includes('offset')?200:0} onError={(message,code)=>{document.body.dataset.error=message?code:'';}}/></div>;}root.render(<App/>);`,loader:'tsx',resolveDir:root},outfile:resolve(temp,'app.js'),bundle:true,format:'esm',platform:'browser',alias:{'@':resolve(root,'src')},define:{'process.env.NODE_ENV':'"production"'}});
let requests=[],aborted=0;
const server=createServer(async(req,res)=>{
 const u=new URL(req.url,'http://localhost');
 if(u.pathname==='/'){res.setHeader('Content-Type','text/html');res.end('<body style="background:black"><div id="root"></div><script type="module" src="/app.js"></script>');return;}
 if(u.pathname==='/app.js'){res.setHeader('Content-Type','text/javascript');res.end(await readFile(resolve(temp,'app.js')));return;}
 if(u.pathname.startsWith('/subtitles/')){res.setHeader('Content-Type',u.pathname.endsWith('.js')?'text/javascript':u.pathname.endsWith('.wasm')?'application/wasm':'application/octet-stream');res.end(await readFile(resolve(root,'public',u.pathname.slice(1))));return;}
 const start=Number(u.searchParams.get('start')),format=u.searchParams.get('format');
 assert.equal(u.searchParams.get('duration'),'120');requests.push({start,format});
 if(start===30&&requests.filter(r=>r.start===30).length===1){res.statusCode=504;res.end('not yet available');return;}
 if(start===90){const timer=setTimeout(()=>res.end('late'),10000);res.on('close',()=>{clearTimeout(timer);aborted++;});return;}
 if(start===270){res.statusCode=502;res.end('unavailable');return;}
 const shift=start===0?0:start===30?60:200;
 res.setHeader('Content-Type',format==='sup'?'application/octet-stream':'text/x-ssa');res.end(format==='sup'?fixture(shift):ass(shift));
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try{
 browser=await chromium.launch({headless:true,args:['--no-sandbox','--enable-unsafe-swiftshader']});
 for(const format of ['pgs','ass']){
  requests=[];aborted=0;
  const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.addInitScript(()=>{
   window.testTime=3.5;
   Object.defineProperty(HTMLMediaElement.prototype,'currentTime',{get(){return window.testTime;}});
   Object.defineProperty(HTMLVideoElement.prototype,'videoWidth',{get(){return 128;}});
   Object.defineProperty(HTMLVideoElement.prototype,'videoHeight',{get(){return 64;}});
   HTMLVideoElement.prototype.requestVideoFrameCallback=function(cb){return setTimeout(()=>cb(performance.now(),{mediaTime:window.testTime,expectedDisplayTime:performance.now(),width:128,height:64}),30);};
  });
  await page.goto(`http://127.0.0.1:${server.address().port}/?${format}`);
  const canvas=page.locator('canvas');await canvas.waitFor();
  async function pixels(){
   for(let i=0;i<30;i++){
    const png=await canvas.screenshot();
    const decoded=spawnSync(process.env.FFMPEG_PATH||'ffmpeg',['-v','error','-f','image2pipe','-i','pipe:0','-f','rawvideo','-pix_fmt','rgb24','pipe:1'],{input:png,maxBuffer:8*1024*1024});
    assert.equal(decoded.status,0,'caption screenshot is decodable');
    let white=0;for(let p=0;p<decoded.stdout.length;p+=3)if(decoded.stdout[p]>230&&decoded.stdout[p+1]>230&&decoded.stdout[p+2]>230)white++;
    if(white>5)return;await page.waitForTimeout(100);
   }
   throw new Error(`${format}: caption pixels missing at ${await page.evaluate(()=>window.testTime)}`);
  }
  await pixels();await canvas.evaluate(el=>window.firstCanvas=el);
  const setTime=async time=>page.locator('video').evaluate((video,time)=>{window.testTime=time;video.dispatchEvent(new Event('timeupdate'));},time);
  await setTime(63.5);
  await page.waitForFunction(()=>document.body.dataset.error==='');
  await page.waitForTimeout(1400);await pixels();
  assert.equal(requests.filter(r=>r.start===30).length,2,'transient failure retries once');
  assert.equal(await canvas.evaluate(el=>el===window.firstCanvas),true,'reuse the caption canvas across windows');
  await setTime(64);await page.waitForTimeout(100);assert.equal(requests.length,3,'time updates within a window do not duplicate downloads');
  await setTime(123.5);await page.waitForTimeout(100);
  assert.equal(requests.at(-1).start,90);
  await setTime(203.5);await page.waitForTimeout(300);await pixels();
  assert.equal(aborted,1,'seek cancels the old chunk request');
  assert.equal(await canvas.evaluate(el=>el===window.firstCanvas),true);
  await setTime(303.5);await page.waitForFunction(()=>document.body.dataset.error==='SUB_HTTP_502');
  assert.equal(requests.filter(r=>r.start===270).length,3,'retries are bounded');
  await setTime(304);await page.waitForTimeout(100);assert.equal(requests.filter(r=>r.start===270).length,3);
  await page.evaluate(()=>window.unmount());assert.equal(await canvas.count(),0,'unmount disposes the subtitle renderer');
  assert.deepEqual(errors,[]);await page.close();
  // Remux seeks rebase video time: chunks still use the absolute episode time.
  requests=[];
  const offset=await browser.newPage();
  await offset.addInitScript(()=>{Object.defineProperty(HTMLMediaElement.prototype,'currentTime',{get(){return 3.5;}});});
  await offset.goto(`http://127.0.0.1:${server.address().port}/?${format}&offset`);
  await offset.locator('canvas').waitFor();assert.equal(requests[0].start,150);
  await offset.close();
 }
 console.log('PASS: visible ASS/PGS cues, original timestamps, automatic bounded retries, no duplicate downloads, seek cancellation, canvas reuse, remux offsets and disposal.');
}finally{await browser?.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(temp,{recursive:true,force:true});}
