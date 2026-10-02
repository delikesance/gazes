import { chromium } from "../web/node_modules/playwright/index.mjs";
import path from "path";

const CHROMIUM_PATH = "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium";
const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity/brain/312ca410-f322-403f-8d41-2aac163c504f";

async function main() {
  console.log("🌐 Launching Chromium browser...");
  const browser = await chromium.launch({
    executablePath: CHROMIUM_PATH,
    headless: true,
    args: ["--no-sandbox", "--disable-setuid-sandbox", "--disable-gpu", "--disable-dev-shm-usage"],
  });

  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
  });

  const page = await context.newPage();
  page.on("console", (msg) => console.log(`[Browser Console ${msg.type()}]:`, msg.text()));
  page.on("pageerror", (err) => console.error("[Browser Page Error]:", err.message));

  console.log("🔗 Navigating to http://127.0.0.1:4389/anime/217331 (Tensura Season 4 / Anime Detail direct route)...");
  await page.goto("http://127.0.0.1:4389/anime/217331", { waitUntil: "networkidle", timeout: 20000 });
  await page.waitForTimeout(3000);

  const screen1 = path.join(ARTIFACT_DIR, "tensura_anime_page.png");
  await page.screenshot({ path: screen1, fullPage: false });
  console.log("📸 Saved screenshot 1:", screen1);

  // Also navigate to Slime Diaries / Tensura Nikki: 116741
  console.log("🔗 Navigating to http://127.0.0.1:4389/anime/116741 (The Slime Diaries - Available Episodes)...");
  await page.goto("http://127.0.0.1:4389/anime/116741", { waitUntil: "networkidle", timeout: 20000 });
  await page.waitForTimeout(3000);

  const screen2 = path.join(ARTIFACT_DIR, "tensura_slime_diaries_page.png");
  await page.screenshot({ path: screen2, fullPage: false });
  console.log("📸 Saved screenshot 2:", screen2);

  // Click on episode sources button
  const sourceBtn = page.locator("button[title*='Choisir une source'], button[title*='source'], button:has-text('Source')").first();
  if (await sourceBtn.isVisible()) {
    console.log("🇫🇷 Clicking on Sources modal for Episode 1...");
    await sourceBtn.click();
    await page.waitForTimeout(3500);

    const screen3 = path.join(ARTIFACT_DIR, "tensura_french_sources_modal.png");
    await page.screenshot({ path: screen3, fullPage: false });
    console.log("📸 Saved screenshot 3:", screen3);
  }

  await browser.close();
  console.log("🎉 Direct anime inspection completed successfully!");
}

main().catch((err) => {
  console.error("❌ Error:", err);
  process.exit(1);
});
