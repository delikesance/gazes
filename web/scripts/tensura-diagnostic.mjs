// Live smoke diagnostic. Reports success only after 30 seconds of advancing video.
import { chromium } from 'playwright';
import { writeFile } from 'node:fs/promises';
const base=process.env.TEST_BASE_URL||'http://127.0.0.1:18080';
const report={started:new Date().toISOString(),base,episode:'101280/101280/1',sessions:[],sources:[],failures:[],errors:[],samples:[],success:false};
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium',args:['--no-sandbox','--autoplay-policy=no-user-gesture-required']});
try{
 const page=await browser.newPage({locale:'fr-FR'});
 page.on('pageerror',e=>report.errors.push(e.message.slice(0,1000)));
 page.on('request',r=>{if(r.url().endsWith('/diagnostics/events')){const d=r.postDataJSON();for(const e of d?.events||[]){if(!report.sessions.includes(e.playback_session_id))report.sessions.push(e.playback_session_id);if(['playback.failed','playback.file_rejected','playback.file_selected','playback.started','playback.media_error'].includes(e.event))report.failures.push(e);}}});
 page.on('response',async r=>{if(new URL(r.url()).pathname.endsWith('/sources')){try{const d=await r.json();if(d.playback_session_id&&!report.sessions.includes(d.playback_session_id))report.sessions.push(d.playback_session_id);report.sources=(d.sources||[]).map(s=>({infohash:s.info_hash,title:s.title,seeders:s.seeders,language:s.language_tag}));}catch{}}});
 await page.goto(`${base}/anime/101280/seasons/101280/episodes/1`,{waitUntil:'domcontentloaded',timeout:60000});
 let first=null,last=null,progress=0;
 const deadline=Date.now()+240000;
 while(Date.now()<deadline){
  await page.waitForTimeout(1000);
  const sample=await page.evaluate(()=>{const v=document.querySelector('video');return v?{position:v.currentTime,width:v.videoWidth,height:v.videoHeight,paused:v.paused,readyState:v.readyState,error:v.error?.code,hash:new URL(v.src).searchParams.get('ih'),fileIndex:new URL(v.src).searchParams.get('file_idx')}:null});
  if(sample){if(sample.hash===last?.hash&&sample.fileIndex===last?.fileIndex&&sample.position>last.position&&sample.width>0){if(!first)first=Date.now();progress+=sample.position-last.position;if(Date.now()-first>=30000&&progress>=29){report.success=true;report.progressSeconds=progress;report.advancementWallSeconds=(Date.now()-first)/1000;last=sample;report.samples.push(sample);report.confirmedFile={hash:sample.hash,fileIndex:sample.fileIndex};break;}}else if(sample.hash!==last?.hash||sample.fileIndex!==last?.fileIndex){first=null;progress=0;}last=sample;}
  if(report.samples.length%10===0)console.log(JSON.stringify({elapsed:Math.round((Date.now()-Date.parse(report.started))/1000),sample,sources:report.sources.length,events:report.failures.length}));
  report.samples.push(sample);
  const text=await page.locator('body').innerText();if(/Toutes les tentatives de lecture ont échoué|Aucun torrent ne correspond|fournisseurs de torrents sont temporairement indisponibles/.test(text)){report.finalMessage=text.slice(-2000);break;}
 }
 report.finalMessage=report.finalMessage||await page.locator('body').innerText();
 console.log(JSON.stringify({success:report.success,sessions:report.sessions,sources:report.sources.length,lastSample:last,errors:report.errors},null,2));
}finally{report.finished=new Date().toISOString();await writeFile(process.env.DIAGNOSTIC_REPORT||'/tmp/gazes-tensura-browser.json',JSON.stringify(report,null,2));await browser.close();}
