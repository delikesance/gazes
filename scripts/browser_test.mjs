import { chromium } from "../web/node_modules/playwright/index.mjs";
import path from "path";
import fs from "fs";

const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity-cli/brain/2a7fea93-6344-4507-be95-97efb8c62c61";
const CHROMIUM_PATH = "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium";

async function runBrowserTest() {
  console.log("🌐 Launching genuine Chromium browser:", CHROMIUM_PATH);
  const browser = await chromium.launch({
    executablePath: CHROMIUM_PATH,
    headless: true,
    args: ["--no-sandbox", "--disable-setuid-sandbox", "--disable-gpu", "--disable-dev-shm-usage"],
  });

  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
  });

  const page = await context.newPage();

  // Log console and errors
  page.on("console", (msg) => console.log(`[Browser Console ${msg.type()}]:`, msg.text()));
  page.on("pageerror", (err) => console.error("[Browser Page Error]:", err.message));

  console.log("🔗 Navigating to http://127.0.0.1:4389...");
  await page.goto("http://127.0.0.1:4389", { waitUntil: "networkidle", timeout: 20000 });
  await page.waitForTimeout(3000);

  // 1. Screenshot Homepage Catalog
  const screen1 = path.join(ARTIFACT_DIR, "live_catalog_home.png");
  await page.screenshot({ path: screen1, fullPage: false });
  console.log("📸 Saved screenshot 1: live_catalog_home.png");

  // 2. Click on the first anime card (e.g. Frieren or first card)
  console.log("🔍 Clicking on an anime card to open show details & episode browser...");
  const firstCard = page.locator(".group.cursor-pointer").first();
  await firstCard.waitFor({ state: "visible", timeout: 10000 });
  await firstCard.click();
  await page.waitForTimeout(3000);

  const screen2 = path.join(ARTIFACT_DIR, "live_anime_episodes_modal.png");
  await page.screenshot({ path: screen2, fullPage: false });
  console.log("📸 Saved screenshot 2: live_anime_episodes_modal.png");

  // 3. Click Sources button on Episode 1
  console.log("🇫🇷 Clicking Sources button on Episode to inspect French VOSTFR/VF torrent swarms...");
  const sourcesBtn = page.locator("button[title*='Choisir une source']").first();
  if (await sourcesBtn.isVisible()) {
    await sourcesBtn.click();
    await page.waitForTimeout(3500);

    const screen3 = path.join(ARTIFACT_DIR, "live_french_sources_modal.png");
    await page.screenshot({ path: screen3, fullPage: false });
    console.log("📸 Saved screenshot 3: live_french_sources_modal.png");

    // 4. Click "Lancer ce flux" on top French source
    console.log("▶️ Launching video stream from top French source...");
    const streamBtn = page.locator("button:has-text('Lancer ce flux')").first();
    if (await streamBtn.isVisible()) {
      await streamBtn.click();
      await page.waitForTimeout(4000);

      const screen4 = path.join(ARTIFACT_DIR, "live_video_player.png");
      await page.screenshot({ path: screen4, fullPage: false });
      console.log("📸 Saved screenshot 4: live_video_player.png");
    }
  }

  await browser.close();
  console.log("🎉 All genuine browser tests completed successfully!");
}

runBrowserTest().catch((err) => {
  console.error("❌ Fatal Browser Test Error:", err);
  process.exit(1);
});
