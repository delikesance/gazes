import { chromium } from '../web/node_modules/playwright/index.mjs';
import path from 'node:path';

const ARTIFACTS_DIR = '/home/nixos/.gemini/antigravity-cli/brain/2a7fea93-6344-4507-be95-97efb8c62c61';
const CHROMIUM_EXEC = '/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium';

async function run() {
  console.log('Starting Playwright test in genuine Chromium...');
  const browser = await chromium.launch({
    executablePath: CHROMIUM_EXEC,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--disable-gpu']
  });

  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } });

  // 1. Open Homepage
  console.log('Navigating to homepage http://127.0.0.1:4389...');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);

  // --- TEST 1: Re:ZERO Season 4 ---
  console.log('--- TEST 1: Re:ZERO Season 4 ---');
  const searchInput = page.locator('input[placeholder*="Rechercher"]').first();
  await searchInput.fill('Re:Zero');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(2000);

  // Find and click Re:ZERO Season 4 or Season 3 card
  const rezeroCard = page.locator('div:has-text("Season 4"), div:has-text("4th Season")').first();
  if (await rezeroCard.isVisible()) {
    console.log('Found Re:ZERO Season 4 card, clicking...');
    await rezeroCard.click();
  } else {
    console.log('Season 4 card not directly visible, clicking first Re:ZERO card...');
    await page.locator('div:has-text("Re:ZERO")').first().click();
  }
  await page.waitForTimeout(2000);

  // Click on Episode 1 in AnimeDetailModal
  const ep1Btn = page.locator('button:has-text("Épisode 1"), button:has-text("1.")').first();
  if (await ep1Btn.isVisible()) {
    console.log('Clicking Episode 1...');
    await ep1Btn.click();
    await page.waitForTimeout(4000);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, '01_rezero_s4_sources.png') });
    console.log('Captured 01_rezero_s4_sources.png');

    // Click on the first source to open video player
    const firstSource = page.locator('button:has-text("Regarder"), button:has-text("Streamer"), div:has-text("Seeders")').first();
    if (await firstSource.isVisible()) {
      console.log('Selecting first source to open player...');
      await firstSource.click();
      await page.waitForTimeout(3000);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '02_rezero_s4_player.png') });
      console.log('Captured 02_rezero_s4_player.png');

      // Close player modal
      const closePlayer = page.locator('button[aria-label="Fermer"], button:has-text("✕"), svg.lucide-x').first();
      if (await closePlayer.isVisible()) {
        await closePlayer.click();
        await page.waitForTimeout(1000);
      }
    }
  }

  // --- TEST 2: One Piece Episode 2 ---
  console.log('--- TEST 2: One Piece Episode 2 ---');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);

  await searchInput.fill('One Piece');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(2000);

  // Click One Piece card
  const opCard = page.locator('div:has-text("One Piece")').first();
  if (await opCard.isVisible()) {
    console.log('Found One Piece card, clicking...');
    await opCard.click();
    await page.waitForTimeout(2000);

    // Look for Episode 2 button
    const ep2Btn = page.locator('button:has-text("Épisode 2"), button:has-text("2")').nth(1);
    if (await ep2Btn.isVisible()) {
      console.log('Clicking Episode 2...');
      await ep2Btn.click();
      await page.waitForTimeout(4000);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '03_onepiece_ep2_sources.png') });
      console.log('Captured 03_onepiece_ep2_sources.png');

      // Close source modal
      const closeSrc = page.locator('button:has-text("Fermer"), button[aria-label="Fermer"], svg.lucide-x').first();
      if (await closeSrc.isVisible()) {
        await closeSrc.click();
        await page.waitForTimeout(1000);
      }
    }
  }

  // --- TEST 3: Mushoku Tensei Last Season ---
  console.log('--- TEST 3: Mushoku Tensei Season 2 Part 2 ---');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);

  await searchInput.fill('Mushoku Tensei');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(2000);

  const mtCard = page.locator('div:has-text("Mushoku Tensei")').first();
  if (await mtCard.isVisible()) {
    console.log('Found Mushoku Tensei card, clicking...');
    await mtCard.click();
    await page.waitForTimeout(2000);

    const mtEp1 = page.locator('button:has-text("Épisode 1"), button:has-text("1.")').first();
    if (await mtEp1.isVisible()) {
      console.log('Clicking Episode 1...');
      await mtEp1.click();
      await page.waitForTimeout(4000);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '04_mushoku_tensei_sources.png') });
      console.log('Captured 04_mushoku_tensei_sources.png');
    }
  }

  await browser.close();
  console.log('Playwright interactive tests completed successfully!');
}

run().catch((err) => {
  console.error('Test execution failed:', err);
  process.exit(1);
});
