import { chromium } from "../web/node_modules/playwright/index.mjs";
import path from "path";

const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity-cli/brain/2a7fea93-6344-4507-be95-97efb8c62c61";
const CHROMIUM_PATH = "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium";

async function testReleasedAnime() {
  const browser = await chromium.launch({
    executablePath: CHROMIUM_PATH,
    headless: true,
    args: ["--no-sandbox", "--disable-setuid-sandbox"],
  });

  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.goto("http://127.0.0.1:4389", { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);

  // Click on ONE PIECE (second card)
  console.log("Clicking on ONE PIECE card...");
  const onePieceCard = page.locator(".group.cursor-pointer:has-text('ONE PIECE')").first();
  await onePieceCard.click();
  await page.waitForTimeout(2500);

  // Open Sources for Episode 1
  console.log("Opening sources for ONE PIECE Ep 1...");
  const sourcesBtn = page.locator("button[title*='Choisir une source']").first();
  await sourcesBtn.click();
  await page.waitForTimeout(3000);

  const sourcesPath = path.join(ARTIFACT_DIR, "live_onepiece_french_sources.png");
  await page.screenshot({ path: sourcesPath, fullPage: false });
  console.log("📸 Saved onepiece sources screenshot:", sourcesPath);

  // Click Lancer ce flux
  console.log("Launching video player...");
  const streamBtn = page.locator("button:has-text('Lancer ce flux')").first();
  await streamBtn.click();
  await page.waitForTimeout(4000);

  const playerPath = path.join(ARTIFACT_DIR, "live_onepiece_video_player.png");
  await page.screenshot({ path: playerPath, fullPage: false });
  console.log("📸 Saved onepiece player screenshot:", playerPath);

  await browser.close();
}

testReleasedAnime().catch(console.error);
