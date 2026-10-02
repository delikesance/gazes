import { chromium } from '../web/node_modules/playwright/index.mjs';
import path from 'node:path';

const ARTIFACTS_DIR = '/home/nixos/.gemini/antigravity/brain/312ca410-f322-403f-8d41-2aac163c504f';
const CHROMIUM_EXEC = '/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium';

async function run() {
  console.log('🚀 Inspecting Tensura search in Gazes...');
  const browser = await chromium.launch({
    executablePath: CHROMIUM_EXEC,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--disable-gpu']
  });

  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } });

  console.log('🔗 Directly navigating to http://127.0.0.1:4389/?q=tensura...');
  await page.goto('http://127.0.0.1:4389/?q=tensura', { waitUntil: 'networkidle' });
  await page.waitForTimeout(3000);

  const debug = await page.evaluate(() => {
    return {
      heading: document.querySelector('.results-heading')?.textContent || 'No heading',
      cardsCount: document.querySelectorAll('.poster-card, a[href*="/anime/"]').length,
      cardTitles: Array.from(document.querySelectorAll('.poster-card h3, .poster-card')).map(el => el.textContent?.trim()).filter(Boolean),
      htmlSnippet: document.querySelector('.catalog-tools')?.innerHTML?.slice(0, 500) || document.body.innerText.slice(0, 500),
    };
  });

  console.log('DOM Evaluation:', JSON.stringify(debug, null, 2));

  const screenshotPath = path.join(ARTIFACTS_DIR, 'tensura_search_capture.png');
  await page.screenshot({ path: screenshotPath, fullPage: true });
  console.log('📸 Screenshot saved to:', screenshotPath);

  await browser.close();
}

run().catch((err) => {
  console.error('❌ Error:', err);
  process.exit(1);
});
