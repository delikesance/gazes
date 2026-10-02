import { chromium } from '../web/node_modules/playwright/index.mjs';
import path from 'node:path';

const ARTIFACTS_DIR = '/home/nixos/.gemini/antigravity/brain/312ca410-f322-403f-8d41-2aac163c504f';
const CHROMIUM_EXEC = '/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium';

async function run() {
  console.log('🚀 Starting Tensura browser inspection with genuine Chromium...');
  const browser = await chromium.launch({
    executablePath: CHROMIUM_EXEC,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--disable-gpu']
  });

  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } });

  // 1. Visit Gazes home
  console.log('🔗 Navigating to http://127.0.0.1:4389...');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);

  // Take screenshot of homepage
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, '01_gazes_homepage.png') });
  console.log('📸 Saved 01_gazes_homepage.png');

  // 2. Open Search toggle if present or fill search input
  console.log('🔍 Searching for "Tensura"...');
  const searchToggle = page.locator('button[aria-label*="Rechercher"], .header-search-toggle').first();
  if (await searchToggle.isVisible()) {
    await searchToggle.click();
    await page.waitForTimeout(500);
  }

  const searchInput = page.locator('input[name="q"], input[placeholder*="Rechercher"]').first();
  if (await searchInput.isVisible()) {
    await searchInput.fill('tensura');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(3000);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, '02_tensura_search_results.png') });
    console.log('📸 Saved 02_tensura_search_results.png');
  }

  // 3. Inspect cards
  const cards = page.locator('.poster-card, a[href*="/anime/"]');
  const count = await cards.count();
  console.log(`Found ${count} anime cards for Tensura`);

  if (count > 0) {
    console.log('🎬 Clicking first Tensura anime card...');
    await cards.first().click();
    await page.waitForTimeout(3500);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, '03_tensura_anime_seasons.png') });
    console.log('📸 Saved 03_tensura_anime_seasons.png');

    // Click on season if available
    const seasonCard = page.locator('.season-card, a[href*="/seasons/"]').first();
    if (await seasonCard.isVisible()) {
      console.log('📺 Opening Season...');
      await seasonCard.click();
      await page.waitForTimeout(3500);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '04_tensura_episodes_list.png') });
      console.log('📸 Saved 04_tensura_episodes_list.png');

      // Click on episode sources
      const sourcesBtn = page.locator('button[title*="Choisir une source"], button[title*="source"]').first();
      if (await sourcesBtn.isVisible()) {
        console.log('🇫🇷 Opening Episode 1 Sources modal...');
        await sourcesBtn.click();
        await page.waitForTimeout(3500);
        await page.screenshot({ path: path.join(ARTIFACTS_DIR, '05_tensura_sources_modal.png') });
        console.log('📸 Saved 05_tensura_sources_modal.png');
      }
    }
  }

  await browser.close();
  console.log('✅ Tensura search and verification finished successfully!');
}

run().catch((err) => {
  console.error('❌ Fatal error:', err);
  process.exit(1);
});
