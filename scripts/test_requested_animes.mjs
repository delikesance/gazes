import { chromium } from '../web/node_modules/playwright/index.mjs';
import path from 'node:path';

const ARTIFACTS_DIR = '/home/nixos/.gemini/antigravity-cli/brain/2a7fea93-6344-4507-be95-97efb8c62c61';
const CHROMIUM_EXEC = '/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium';

async function run() {
  console.log('Launching genuine Chromium...');
  const browser = await chromium.launch({
    executablePath: CHROMIUM_EXEC,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--disable-gpu']
  });

  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

  // 1. Test Re:ZERO Season 4 (ID 189046)
  console.log('1. Testing Re:ZERO Season 4 (ID: 189046)...');
  await page.goto('http://127.0.0.1:4389/anime/189046', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // Click on Episode 1
  const ep1Button = page.locator('button:has-text("Épisode 1"), button:has-text("Episode 1")').first();
  if (await ep1Button.isVisible()) {
    await ep1Button.click();
    console.log('Clicked Re:ZERO S4 Episode 1');
    await page.waitForTimeout(4000); // Wait for sources resolution
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'test_rezero_s4_sources.png') });
    console.log('Saved test_rezero_s4_sources.png');
  } else {
    console.log('Episode 1 button not found directly, taking page screenshot');
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'test_rezero_s4_sources.png') });
  }

  // 2. Test One Piece (ID 21) - Episode 2
  console.log('2. Testing One Piece (ID: 21) Episode 2...');
  await page.goto('http://127.0.0.1:4389/anime/21', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  const ep2Button = page.locator('button:has-text("Épisode 2"), button:has-text("Episode 2")').first();
  if (await ep2Button.isVisible()) {
    await ep2Button.click();
    console.log('Clicked One Piece Episode 2');
    await page.waitForTimeout(4000); // Wait for sources
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'test_onepiece_ep2_sources.png') });
    console.log('Saved test_onepiece_ep2_sources.png');
  }

  // 3. Test Mushoku Tensei Season 2 Part 2 (ID 166873)
  console.log('3. Testing Mushoku Tensei Season 2 Part 2 (ID: 166873)...');
  await page.goto('http://127.0.0.1:4389/anime/166873', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  const mtEp1Button = page.locator('button:has-text("Épisode 1"), button:has-text("Episode 1")').first();
  if (await mtEp1Button.isVisible()) {
    await mtEp1Button.click();
    console.log('Clicked Mushoku Tensei Episode 1');
    await page.waitForTimeout(4000); // Wait for sources
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'test_mushoku_tensei_sources.png') });
    console.log('Saved test_mushoku_tensei_sources.png');
  }

  // 4. Test Search Page with grouping and 0-seed filtering
  console.log('4. Testing search with grouping...');
  await page.goto('http://127.0.0.1:4389/?q=Mushoku', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'test_mushoku_search_catalog.png') });
  console.log('Saved test_mushoku_search_catalog.png');

  await browser.close();
  console.log('All tests finished successfully!');
}

run().catch((err) => {
  console.error('Test error:', err);
  process.exit(1);
});
