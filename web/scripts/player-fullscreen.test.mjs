// Exercise the real browser fullscreen layer with the rendered player and CSS.
import { build } from 'esbuild';
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { createRequire } from 'node:module';
import assert from 'node:assert/strict';

const root = resolve(import.meta.dirname, '..');
const temporary = await mkdtemp(resolve(tmpdir(), 'gazes-fullscreen-'));
const require = createRequire(import.meta.url);
const postcss = createRequire(require.resolve('@tailwindcss/postcss'))('postcss');
const { default: tailwind } = await import('@tailwindcss/postcss');
const css = await postcss([tailwind({ base: root })]).process(await readFile(resolve(root, 'src/app/globals.css'), 'utf8'), { from: resolve(root, 'src/app/globals.css') });
const meta = { duration_sec: 120, video_codec: 'h264', audio_tracks: [], subtitle_tracks: [] };
const item = { id: 'test', info_hash: 'test', magnet_uri: 'magnet:?xt=urn:btih:test', title: 'Fullscreen fixture', episode_number: 1, season_number: 1, anime_aliases: ['Test'] };
await build({ absWorkingDir: root, stdin: { contents: `import React from 'react';import {createRoot} from 'react-dom/client';import {VideoPlayerModal} from './src/components/VideoPlayerModal';createRoot(document.getElementById('root')).render(<VideoPlayerModal item={${JSON.stringify(item)}} animeTitle="Fullscreen fixture" episodeNumber={1} pageMode={location.search.includes('page')} onClose={()=>document.body.dataset.closed='true'} onChangeSource={()=>document.body.dataset.sources='true'}/>);`, loader: 'tsx', resolveDir: root }, outfile: resolve(temporary, 'player.js'), bundle: true, format: 'esm', platform: 'browser', alias: { '@': resolve(root, 'src') }, define: { 'process.env.NODE_ENV': '"production"', 'process.env.NEXT_PUBLIC_API_BASE': '"/api/v1"' } });
const server = createServer(async (request, response) => {
  const url = new URL(request.url, 'http://localhost');
  if (url.pathname === '/') { response.setHeader('Content-Type', 'text/html'); response.end('<link rel="stylesheet" href="/styles.css"><div id="root"></div><script type="module" src="/player.js"></script>'); }
  else if (url.pathname === '/styles.css') { response.setHeader('Content-Type', 'text/css'); response.end(css.css); }
  else if (url.pathname === '/player.js') { response.setHeader('Content-Type', 'text/javascript'); response.end(await readFile(resolve(temporary, 'player.js'))); }
  else if (url.pathname.endsWith('/stream')) { response.setHeader('Content-Type', 'video/mp4'); response.flushHeaders(); }
  else {
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify(url.pathname.endsWith('/torrent/load') ? { info_hash: 'test', files: [{ index: 0, path: 'Test - 01.mkv', is_video: true }], main_video_index: 0, main_video_metadata: meta } : url.pathname.endsWith('/metadata') ? meta : { active_seeders: 3, download_rate_bps: 0 }));
  }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
let browser;
try {
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {}), args: ['--no-sandbox'] });
  for (const mode of ['page', 'modal']) {
    const page = await browser.newPage({ locale: 'fr-FR', viewport: { width: 1912, height: 914 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.clock.install();
    await page.addInitScript(() => {
      HTMLMediaElement.prototype.play = function () { this.dispatchEvent(new Event('play')); return Promise.resolve(); };
      HTMLMediaElement.prototype.pause = function () { this.dispatchEvent(new Event('pause')); };
    });
    await page.goto(`http://127.0.0.1:${server.address().port}/?${mode}`);
    await page.locator('video').waitFor();
    await page.locator('video').evaluate(video => { window.originalVideo = video; video.dispatchEvent(new Event('canplay')); });
    const bar = page.locator('.player-topbar');
    const fullscreen = page.getByRole('button', { name: 'Plein écran (F)', exact: true });
    await fullscreen.click();
    await page.waitForFunction(() => Boolean(document.fullscreenElement));
    assert.equal(await bar.evaluate(element => document.fullscreenElement.contains(element)), true, `${mode}: top bar belongs to the visible fullscreen layer`);
    assert.equal(await bar.isVisible(), true);
    await page.getByRole('button', { name: 'Changer de source pour cet épisode', exact: true }).click();
    assert.equal(await page.locator('body').getAttribute('data-sources'), 'true', 'source button is actually clickable in fullscreen');
    await page.mouse.move(500, 400);
    await page.clock.fastForward(2600);
    assert.equal(await bar.isVisible(), false, 'top bar fades with the dock during playback');
    assert.equal(await page.locator('[data-player-controls]').isVisible(), false);
    await page.mouse.move(510, 410);
    await bar.waitFor({ state: 'visible' });
    await fullscreen.click();
    await page.waitForFunction(() => !document.fullscreenElement);
    assert.equal(await bar.count(), 1, 'returning from fullscreen restores a single top bar');
    assert.equal(await bar.isVisible(), true);
    assert.equal(await page.locator('video').evaluate(video => video === window.originalVideo), true, 'fullscreen must preserve the current video');
    await page.setViewportSize({ width: 430, height: 800 });
    await fullscreen.click();
    await page.waitForFunction(() => Boolean(document.fullscreenElement));
    const box = await bar.boundingBox();
    assert.ok(box.x >= 0 && box.x + box.width <= 430, 'top bar fits a narrow fullscreen viewport');
    await fullscreen.click();
    await page.getByRole('button', { name: mode === 'page' ? 'Voir les saisons' : 'Fermer le lecteur', exact: true }).click();
    assert.equal(await page.locator('body').getAttribute('data-closed'), 'true');
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log('PASS: page and modal fullscreen, clickable top bar, hide/wake, exit, narrow screens, preserved video, and return/close.');
} finally {
  await browser?.close();
  server.closeAllConnections();
  await new Promise(resolve => server.close(resolve));
  await rm(temporary, { recursive: true, force: true });
}
