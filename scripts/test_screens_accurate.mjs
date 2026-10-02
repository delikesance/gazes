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

  // 1. TEST RE:ZERO (Season 4 / Season 3)
  console.log('1. Testing Re:ZERO...');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);

  const searchInput = page.locator('input[placeholder*="Rechercher"]').first();
  await searchInput.fill('Re:Zero');
  await page.locator('button:has-text("Rechercher")').first().click();
  await page.waitForTimeout(2500);

  // Click on the first "Voir les épisodes" button
  const rezeroBtn = page.locator('button:has-text("Voir les épisodes")').first();
  if (await rezeroBtn.isVisible()) {
    console.log('Clicking Re:ZERO episodes...');
    await rezeroBtn.click();
    await page.waitForTimeout(2000);

    // Click on sources button (Layers icon) on episode 1
    const srcBtn = page.locator('button[title*="Choisir une source"]').first();
    if (await srcBtn.isVisible()) {
      console.log('Opening Re:ZERO Episode 1 sources modal...');
      await srcBtn.click();
      await page.waitForTimeout(4000);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '01_rezero_sources_modal.png') });
      console.log('Saved 01_rezero_sources_modal.png');

      // Click first source to play
      const firstStreamBtn = page.locator('button:has-text("Regarder"), button:has-text("Streamer")').first();
      if (await firstStreamBtn.isVisible()) {
        await firstStreamBtn.click();
        await page.waitForTimeout(3500);
        await page.screenshot({ path: path.join(ARTIFACTS_DIR, '02_rezero_player.png') });
        console.log('Saved 02_rezero_player.png');
      }
    }
  }

  // 2. TEST ONE PIECE (Episode 2)
  console.log('2. Testing One Piece Episode 2...');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);

  await searchInput.fill('One Piece');
  await page.locator('button:has-text("Rechercher")').first().click();
  await page.waitForTimeout(2500);

  const opBtn = page.locator('button:has-text("Voir les épisodes")').first();
  if (await opBtn.isVisible()) {
    console.log('Clicking One Piece episodes...');
    await opBtn.click();
    await page.waitForTimeout(2000);

    // Click source button for episode 2 (index 1)
    const opSources = page.locator('button[title*="Choisir une source"]');
    if (await opSources.count() > 1) {
      console.log('Opening One Piece Episode 2 sources modal...');
      await opSources.nth(1).click();
      await page.waitForTimeout(4000);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '03_onepiece_ep2_sources_modal.png') });
      console.log('Saved 03_onepiece_ep2_sources_modal.png');
    }
  }

  // 3. TEST MUSHOKU TENSEI (Last Season / Season 2 Part 2)
  console.log('3. Testing Mushoku Tensei Season 2 Part 2...');
  await page.goto('http://127.0.0.1:4389', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);

  await searchInput.fill('Mushoku Tensei');
  await page.locator('button:has-text("Rechercher")').first().click();
  await page.waitForTimeout(2500);

  const mtBtns = page.locator('button:has-text("Voir les épisodes")');
  if (await mtBtns.count() > 0) {
    const btnToClick = (await mtBtns.count() >= 3) ? mtBtns.nth(2) : mtBtns.first();
    console.log('Clicking Mushoku Tensei episodes...');
    await btnToClick.click();
    await page.waitForTimeout(2000);

    const mtSrcBtn = page.locator('button[title*="Choisir une source"]').first();
    if (await mtSrcBtn.isVisible()) {
      console.log('Opening Mushoku Tensei Episode 1 sources modal...');
      await mtSrcBtn.click();
      await page.waitForTimeout(4000);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, '04_mushoku_sources_modal.png') });
      console.log('Saved 04_mushoku_sources_modal.png');
    }
  }

  await browser.close();
  console.log('All tests completed successfully!');
}

run().catch((err) => {
  console.error('Error during test execution:', err);
  process.exit(1);
});
