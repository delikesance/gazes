// Real modal + JASSUB with a simulated clock; this does not prove HEVC decoding.
import { build } from 'esbuild';
import { firefox, chromium } from 'playwright';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { createRequire } from 'node:module';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
const root = resolve(import.meta.dirname, '..');
const temporary = await mkdtemp(resolve(tmpdir(), 'gazes-player-'));
const require = createRequire(import.meta.url);
const postcss = createRequire(require.resolve('@tailwindcss/postcss'))('postcss');
const { default: tailwind } = await import('@tailwindcss/postcss');
const css = await postcss([tailwind({ base: root })]).process(await readFile(resolve(root, 'src/app/globals.css'), 'utf8'), { from: resolve(root, 'src/app/globals.css') });
const meta = { duration_sec: 120, video_codec: 'hevc', width: 1280, height: 720, total_bytes: 1000, audio_tracks: [{ index: 0, title: 'English', language: 'eng' }, { index: 1, title: 'Japanese', language: 'jpn' }], subtitle_tracks: [{ index: 0, title: 'French', language: 'fra', codec: 'ass', is_default: true }] };
const item = { id: 'test', info_hash: 'test', magnet_uri: 'magnet:?xt=urn:btih:test', title: 'HEVC fixture', episode_number:1,season_number:1,anime_aliases:['test'], size_bytes: 1000, seeders: 1 };
const ass = `[Script Info]
ScriptType: v4.00+
PlayResX: 1280
PlayResY: 720
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Liberation Sans,52,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,20,20,30,1
[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:01:00.00,Default,,0,0,0,,Sous-titres toujours visibles
`;
await build({ absWorkingDir: root, stdin: { contents: `import React from 'react'; import { createRoot } from 'react-dom/client'; import { VideoPlayerModal } from './src/components/VideoPlayerModal'; createRoot(document.getElementById('root')).render(<VideoPlayerModal item={${JSON.stringify(item)}} animeTitle="Test anime" onClose={()=>{}}/>);`, loader: 'tsx', resolveDir: root }, outfile: resolve(temporary, 'player.js'), bundle: true, format: 'esm', platform: 'browser', alias: { '@': resolve(root, 'src') }, define: { 'process.env.NODE_ENV': '"production"', 'process.env.NEXT_PUBLIC_API_BASE': '"/api/v1"' } });
const server = createServer(async (request, response) => {
 try {
 const url = new URL(request.url, 'http://localhost');
 if (url.pathname === '/') { response.setHeader('Content-Type', 'text/html'); response.end('<link rel="stylesheet" href="/styles.css"><div id="root"></div><script type="module" src="/player.js"></script>'); }
 else if (url.pathname === '/styles.css') { response.setHeader('Content-Type', 'text/css'); response.end(css.css); }
 else if (url.pathname === '/player.js') { response.setHeader('Content-Type', 'text/javascript'); response.end(await readFile(resolve(temporary, 'player.js'))); }
 else if (url.pathname.startsWith('/subtitles/')) { response.setHeader('Content-Type', url.pathname.endsWith('.wasm') ? 'application/wasm' : url.pathname.endsWith('.js') ? 'text/javascript' : 'application/octet-stream'); response.end(await readFile(resolve(root, 'public', url.pathname.slice(1)))); }
 else if (url.pathname.endsWith('/torrent/load')) { response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify({ info_hash: 'test', files: [{ index: 0, path: 'test - 01 1080p.mkv', length: 1000, is_video: true },{index:1,path:'test - 01 720p.mkv',length:800,is_video:true},{index:2,path:'test - 02 480p.mkv',length:500,is_video:true}], main_video_index: 0, main_video_metadata: meta })); }
 else if (url.pathname.endsWith('/metadata')) { response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify(meta)); }
 else if (url.pathname.endsWith('/subtitles')) { assert.equal(url.searchParams.get('format'), 'ass'); response.end(ass); }
 else if (url.pathname.endsWith('/stream')) { response.setHeader('Content-Type', 'video/mp4'); response.flushHeaders(); request.on('close', ()=>response.end()); }
 else { response.setHeader('Content-Type', 'application/json'); response.end('{}'); }
 } catch(error) { response.statusCode=500; response.end(String(error)); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
let browser;
try {
 browser = process.env.PLAYWRIGHT_BROWSER === 'chromium' ? await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium',args:['--no-sandbox','--enable-unsafe-swiftshader']}) : await firefox.launch({ headless: true, ...(process.env.FIREFOX_PATH ? { executablePath: process.env.FIREFOX_PATH } : {}) });
 const page = await browser.newPage({ locale:'fr-FR', viewport: { width: 1280, height: 900 } });
 const errors=[]; page.on('pageerror', error=>errors.push(String(error)));
 const capabilities = await page.evaluate(()=>['hvc1.1.6.L93.B0','hvc1.2.4.L123.B0'].map(codec=>document.createElement('video').canPlayType(`video/mp4; codecs="${codec}"`)));
 console.log('Firefox HEVC 8/10-bit:', capabilities);
 if (capabilities.some(value=>!value)) console.log('BLOCKED: native Firefox HEVC playback cannot be verified in this environment.');
 await page.addInitScript(() => {
  window.testTime = 2;
  window.playCalls = 0;
  HTMLMediaElement.prototype.canPlayType = () => 'probably';
  HTMLMediaElement.prototype.play = function() { window.playCalls++; this.testPaused=false; this.dispatchEvent(new Event('play')); return Promise.resolve(); };
  HTMLMediaElement.prototype.pause = function() { this.testPaused=true; this.dispatchEvent(new Event('pause')); };
  Object.defineProperty(HTMLMediaElement.prototype, 'paused', { get() { return this.testPaused !== false; } });
  Object.defineProperty(HTMLMediaElement.prototype, 'currentTime', { get() { return window.testTime; }, set(value) { window.testTime=value; } });
  Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { get:()=>1280 });
  Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { get:()=>720 });
  HTMLVideoElement.prototype.requestVideoFrameCallback = function(callback) { return setTimeout(()=>callback(performance.now(), { mediaTime: window.testTime, expectedDisplayTime: performance.now(), width: 1280, height: 720 }), 50); };
 });
 await page.goto(`http://127.0.0.1:${server.address().port}`);
 await page.waitForSelector('canvas.JASSUB');
 await page.locator('video').evaluate(video=>video.dispatchEvent(new Event('canplay')));
 await page.waitForTimeout(2000);
 assert.equal(await page.getByRole('alert').count(), 0);
 const canvas = page.locator('canvas.JASSUB');
 let first;
 for (let attempt=0; attempt<30; attempt++) {
  first = await canvas.screenshot();
  const decoded = spawnSync(process.env.FFMPEG_PATH || 'ffmpeg', ['-v','error','-f','image2pipe','-i','pipe:0','-f','rawvideo','-pix_fmt','rgb24','pipe:1'], {input:first,maxBuffer:8*1024*1024});
  assert.equal(decoded.status,0,'screenshot must be decodable');
  let white=0;
  for(let i=0;i<decoded.stdout.length;i+=3) if(decoded.stdout[i]>240 && decoded.stdout[i+1]>240 && decoded.stdout[i+2]>240) white++;
  if(white>100) break;
  if(attempt===29) throw new Error('JASSUB did not render caption pixels');
  await page.waitForTimeout(250);
 }
 await page.locator('[data-player-stage]').hover();
 await page.mouse.move(0,0);
 await page.waitForFunction(()=>getComputedStyle(document.querySelector('[data-player-controls]')).visibility==='hidden');
 assert.equal(await canvas.evaluate(el=>getComputedStyle(el).opacity), '1');
 const after = await canvas.screenshot();
 const pixels = spawnSync(process.env.FFMPEG_PATH || 'ffmpeg', ['-v','error','-f','image2pipe','-i','pipe:0','-f','rawvideo','-pix_fmt','rgb24','pipe:1'], {input:after,maxBuffer:8*1024*1024});
 let visibleWhite=0;
 for(let i=0;i<pixels.stdout.length;i+=3) if(pixels.stdout[i]>240 && pixels.stdout[i+1]>240 && pixels.stdout[i+2]>240) visibleWhite++;
 assert.ok(visibleWhite>100,'caption pixels must remain after controls fade');
 const [captionBox, controlsBox] = await Promise.all([canvas.boundingBox(), page.locator('[data-player-controls]').boundingBox()]);
 assert.ok(captionBox.y+captionBox.height <= controlsBox.y+1, 'captions must not overlap controls');
 await page.locator('[data-player-stage]').hover();
 await page.getByTitle('Choisir la piste audio').click();
 await page.getByRole('button', { name:'Japanese', exact:true }).click();
 await page.waitForFunction(()=>document.querySelector('video').src.includes('audio_track=1'));
 await page.waitForSelector('canvas.JASSUB');
 await page.locator('video').evaluate(video=>video.dispatchEvent(new Event('canplay')));
 await page.waitForTimeout(1000);
 assert.equal(await canvas.count(), 1, 'old renderers must be removed');
 await page.locator('input[type="range"]').fill('0.4');
 await page.getByTitle('Couper le son (M)',{exact:true}).click();
 await page.getByTitle('Pause (Espace)',{exact:true}).click();
 const playsBefore = await page.evaluate(()=>window.playCalls);
 await page.getByTitle('Choisir la piste audio').click();
 await page.getByRole('button', {name:'English',exact:true}).click();
 await page.waitForFunction(()=>!document.querySelector('video').src.includes('audio_track=1'));
 await page.locator('video').evaluate(video=>video.dispatchEvent(new Event('canplay')));
 assert.equal(await page.evaluate(()=>window.playCalls), playsBefore, 'changing audio while paused must not start playback');
 assert.equal(await page.locator('video').evaluate(video=>video.volume), 0.4);
 assert.equal(await page.locator('video').evaluate(video=>video.muted), true);
 await page.getByTitle('Lecture (Espace)',{exact:true}).click();
 await page.getByTitle('Choisir les sous-titres').click();
 await page.getByRole('button',{name:'Désactivés',exact:true}).click();
 await page.waitForFunction(()=>!document.querySelector('canvas.JASSUB'));
 await page.getByTitle('Choisir les sous-titres').click();
 await page.getByRole('button',{name:'French',exact:true}).click();
 await page.waitForSelector('canvas.JASSUB');
 await page.setViewportSize({width:900,height:700});
 await page.waitForTimeout(500);
 assert.equal(await page.getByRole('alert').count(),0);
 await page.getByTitle('Plein écran (F)').click();
 await page.waitForFunction(()=>Boolean(document.fullscreenElement));
 await page.waitForTimeout(500);
 const [fullCanvas, fullControls] = await Promise.all([canvas.boundingBox(), page.locator('[data-player-controls]').boundingBox()]);
 assert.ok(fullCanvas.y+fullCanvas.height <= fullControls.y+1, 'fullscreen captions must not overlap controls');
 await page.getByTitle('Plein écran (F)').click();
 await page.waitForFunction(()=>!document.fullscreenElement);
 await page.evaluate(()=>{localStorage.setItem('gazes-language','en');window.dispatchEvent(new Event('gazes-language-change'));});
 await page.getByTitle('Select Audio Track').waitFor();
 await page.getByTitle('Select Subtitles').waitFor();
 assert.equal(await page.locator('canvas.JASSUB').count(),1,'switching UI language must preserve subtitle rendering');
 assert.equal(await page.locator('.player-heading').innerText(),'Test anime');
 await page.mouse.move(450,350);
 await page.getByRole('button',{name:'Quality',exact:true}).click();
 assert.equal(await page.getByRole('button',{name:'480p',exact:true}).count(),0,'other episodes must not appear as qualities');
 await page.getByRole('button',{name:'720p',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('video').src.includes('file_idx=1'));
 const qualityURL=new URL(await page.locator('video').getAttribute('src'),'http://localhost');
 assert.ok(Number(qualityURL.searchParams.get('time_offset'))>0,'quality change preserves absolute position');
 await page.locator('video').evaluate(video=>video.dispatchEvent(new Event('canplay')));
 await page.waitForSelector('canvas.JASSUB');
 assert.equal(await page.locator('video').evaluate(video=>video.volume),0.4);
 assert.deepEqual(errors,[]);
 console.log('PASS: JASSUB, controls fade, layout, source replacement, subtitles off/on, volume/mute/pause, resize and fullscreen (simulated clock).');
} finally { await browser?.close(); server.closeAllConnections(); await new Promise(resolve=>server.close(resolve)); await rm(temporary,{recursive:true,force:true}); }
