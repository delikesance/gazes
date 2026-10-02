import assert from 'node:assert/strict';
import { chromium } from 'playwright';
const base = process.env.TEST_BASE_URL || 'http://127.0.0.1:3000';
const browser = await chromium.launch({
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined,
  args: ['--no-sandbox'],
});
const page = await browser.newPage({ viewport: { width: 847, height: 884 }, locale: 'fr-FR' });
const svg = color => `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="1900" height="650"><rect width="100%" height="100%" fill="${color}"/></svg>`)}`;
try {
  // Exercise the SSR artwork, including a reload with already-cached images.
  for (let attempt = 0; attempt < 2; attempt++) {
    await page.goto(base);
    await page.waitForFunction(() => {
      const image = document.querySelector('.hero-wide img');
      return image?.naturalWidth > 0 && getComputedStyle(image).opacity === '1';
    });
  }
  const items = [
    { id: 900, media_id: 901, title_romaji: 'Featured one', display_title: 'Featured one', status: 'FINISHED', banner_image: svg('#946d35'), poster_image: svg('#355694') },
    { id: 902, media_id: 903, title_romaji: 'Featured two', display_title: 'Featured two', status: 'RELEASING', banner_image: `${base}/broken-banner.jpg`, poster_image: svg('#94355c') },
    { id: 904, display_title: 'Upcoming', status: 'NOT_YET_RELEASED', banner_image: svg('#000') },
  ];
  await page.route('**/broken-banner.jpg', route => route.fulfill({ status: 404 }));
  await page.route('**/api/v1/catalog/**', route => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items, page: 1, has_next_page: false }) }));
  await page.goto(base);
  await page.getByRole('heading', { name: 'Featured one', exact: true, level: 1 }).waitFor();
  await page.mouse.move(0, 880);
  await page.getByRole('heading', { name: 'Featured two', exact: true, level: 1 }).waitFor({ timeout: 12000 });
  await page.waitForFunction(() => {
    const image = document.querySelector('.hero-wide img');
    return image?.src.startsWith('data:') && image.naturalWidth > 0 && getComputedStyle(image).opacity === '1';
  });
  assert.equal(await page.locator('.hero-actions a').first().getAttribute('href'), '/anime/902/seasons/903/episodes/1');
  assert.equal(await page.locator('.featured-controls span').innerText(), '2 / 2');
  await page.getByRole('button', { name: 'Anime suivant', exact: true }).click();
  await page.getByRole('heading', { name: 'Featured one', exact: true, level: 1 }).waitFor();
  await page.mouse.move(0, 880);
  await page.waitForTimeout(8500);
  assert.equal(await page.locator('.anime-hero h1').innerText(), 'Featured one', 'manual selection pauses automatic rotation');
  await page.setViewportSize({ width: 430, height: 900 });
  await page.waitForFunction(() => {
    const image = document.querySelector('.hero-portrait img');
    return image?.naturalWidth > 0 && getComputedStyle(image).opacity === '1';
  });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.getByRole('button', { name: 'Reprendre le défilement', exact: true }).click();
  await page.mouse.move(0, 880);
  await page.locator('.featured-controls button').last().blur();
  await page.waitForTimeout(8500);
  assert.equal(await page.locator('.anime-hero h1').innerText(), 'Featured one', 'reduced motion prevents automatic rotation');
  console.log('PASS: cached SSR artwork, automatic rotation, image fallback, matching playback links, manual controls, mobile artwork, reduced motion.');
} finally {
  await browser.close();
}
