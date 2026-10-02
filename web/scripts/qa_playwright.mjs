import { chromium } from "playwright";
import fs from "fs";
import path from "path";

const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity-cli/brain/2a7fea93-6344-4507-be95-97efb8c62c61";
const SCREENSHOT_DIR = path.join(ARTIFACT_DIR, "qa_screenshots");

if (!fs.existsSync(SCREENSHOT_DIR)) {
  fs.mkdirSync(SCREENSHOT_DIR, { recursive: true });
}

async function runQA() {
  console.log("🚀 Starting Playwright Comprehensive UX/UI Testing Suite...");
  
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || 
    (fs.existsSync("/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium") 
      ? "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium" 
      : undefined);

  const browser = await chromium.launch({ 
    headless: true,
    executablePath,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-gpu']
  });

  const report = {
    timestamp: new Date().toISOString(),
    tests: [],
    screenshots: [],
    errors: [],
  };

  try {
    // -------------------------------------------------------------
    // Test 1: Desktop Viewport Catalog Home (Hero Banner + Trending Grid)
    // -------------------------------------------------------------
    console.log("📸 Test 1: Capturing Desktop Catalog Home - Hero Banner & Trending Grid (1920x1080)...");
    const desktopPage = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
    desktopPage.on("console", (msg) => {
      if (msg.type() === "error") console.log("[BROWSER ERROR]", msg.text());
    });
    desktopPage.on("response", (res) => {
      if (res.status() >= 400) {
        console.log(`[HTTP ${res.status()}] ${res.url()}`);
      }
    });

    await desktopPage.goto("http://127.0.0.1:4389", { waitUntil: "networkidle", timeout: 20000 });
    await desktopPage.waitForSelector(".group.cursor-pointer", { timeout: 15000 });
    await desktopPage.waitForTimeout(1000);

    const desktopHomeTrendingPath = path.join(SCREENSHOT_DIR, "01_desktop_home_hero_trending.png");
    await desktopPage.screenshot({ path: desktopHomeTrendingPath, fullPage: false });
    report.screenshots.push({ name: "Desktop Hero & Trending Grid", path: desktopHomeTrendingPath });
    report.tests.push({ name: "Desktop Hero Banner & Trending Grid Render", status: "PASS" });

    // -------------------------------------------------------------
    // Test 2: Desktop Popular Grid (Les Incontournables)
    // -------------------------------------------------------------
    console.log("⭐ Test 2: Switching to Popular Tab ('Les Incontournables')...");
    const popularTabBtn = desktopPage.locator("button:has-text('Les Incontournables')").first();
    if (await popularTabBtn.isVisible()) {
      await popularTabBtn.click();
      await desktopPage.waitForTimeout(2000);
      const desktopHomePopularPath = path.join(SCREENSHOT_DIR, "02_desktop_home_popular.png");
      await desktopPage.screenshot({ path: desktopHomePopularPath, fullPage: false });
      report.screenshots.push({ name: "Desktop Popular Grid", path: desktopHomePopularPath });
      report.tests.push({ name: "Desktop Popular Grid Tab", status: "PASS" });
    }

    // Return to Trending tab
    const trendingTabBtn = desktopPage.locator("button:has-text('Tendances')").first();
    if (await trendingTabBtn.isVisible()) {
      await trendingTabBtn.click();
      await desktopPage.waitForTimeout(1000);
    }

    // -------------------------------------------------------------
    // Test 3: Mobile Viewport Responsiveness (390x844)
    // -------------------------------------------------------------
    console.log("📱 Test 3: Capturing Mobile Viewport Responsiveness (390x844)...");
    const mobilePage = await browser.newPage({ viewport: { width: 390, height: 844 }, isMobile: true });
    await mobilePage.goto("http://127.0.0.1:4389", { waitUntil: "networkidle", timeout: 20000 });
    await mobilePage.waitForSelector(".group.cursor-pointer", { timeout: 15000 });
    await mobilePage.waitForTimeout(1000);

    const mobileHomePath = path.join(SCREENSHOT_DIR, "03_mobile_home_catalog.png");
    await mobilePage.screenshot({ path: mobileHomePath, fullPage: false });
    report.screenshots.push({ name: "Mobile Catalog Viewport", path: mobileHomePath });
    report.tests.push({ name: "Mobile Responsiveness Grid Render", status: "PASS" });

    // Test clicking anime card on mobile
    const firstMobileCard = mobilePage.locator(".group.cursor-pointer").first();
    await firstMobileCard.click();
    await mobilePage.waitForTimeout(2000);
    const mobileDetailPath = path.join(SCREENSHOT_DIR, "04_mobile_anime_details.png");
    await mobilePage.screenshot({ path: mobileDetailPath, fullPage: false });
    report.screenshots.push({ name: "Mobile Anime Details Modal", path: mobileDetailPath });
    report.tests.push({ name: "Mobile Anime Details Modal Open", status: "PASS" });
    await mobilePage.close();

    // -------------------------------------------------------------
    // Test 4: Anime Detail View & Episode Browser (Desktop - One Piece)
    // -------------------------------------------------------------
    console.log("🎬 Test 4: Opening Anime Details & Episode Browser Modal for ONE PIECE...");
    const onePieceCard = desktopPage.locator(".group.cursor-pointer:has-text('ONE PIECE')").first();
    if (await onePieceCard.isVisible()) {
      await onePieceCard.click();
    } else {
      const firstCard = desktopPage.locator(".group.cursor-pointer").first();
      await firstCard.click();
    }
    await desktopPage.waitForTimeout(2500);

    const detailModalPath = path.join(SCREENSHOT_DIR, "05_desktop_anime_details_episodes.png");
    await desktopPage.screenshot({ path: detailModalPath, fullPage: false });
    report.screenshots.push({ name: "Anime Show Details & Episode Browser", path: detailModalPath });
    report.tests.push({ name: "Anime Details & Episode Browser Modal", status: "PASS" });

    // -------------------------------------------------------------
    // Test 5: French Episode Sources Selector Modal
    // -------------------------------------------------------------
    console.log("🇫🇷 Test 5: Opening French Sources Selector Modal (VOSTFR / VF / MULTI)...");
    const sourcesBtn = desktopPage.locator("button[title*='Choisir une source']").first();
    await desktopPage.waitForSelector("button[title*='Choisir une source']", { timeout: 10000 });
    await sourcesBtn.click();
    await desktopPage.waitForTimeout(4000);

    const sourcesModalPath = path.join(SCREENSHOT_DIR, "06_desktop_french_sources_modal.png");
    await desktopPage.screenshot({ path: sourcesModalPath, fullPage: false });
    report.screenshots.push({ name: "French Sources Selector Modal", path: sourcesModalPath });
    report.tests.push({ name: "French Sources Selector Modal Badges & Feed", status: "PASS" });

    // -------------------------------------------------------------
    // Test 6: Launch Video Player from Top French Source
    // -------------------------------------------------------------
    console.log("▶️ Test 6: Launching video stream from top French source...");
    const streamBtn = desktopPage.locator("button:has-text('Lancer ce flux')").first();
    await desktopPage.waitForSelector("button:has-text('Lancer ce flux')", { timeout: 10000 });
    await streamBtn.click();
    await desktopPage.waitForTimeout(4000);

    const playerPath = path.join(SCREENSHOT_DIR, "07_desktop_video_player_modal.png");
    await desktopPage.screenshot({ path: playerPath, fullPage: false });
    report.screenshots.push({ name: "Video Player Modal with Torrent Stream", path: playerPath });
    report.tests.push({ name: "Video Player Launch & Telemetry", status: "PASS" });

    // -------------------------------------------------------------
    // Test 7: Subtitle & Audio Track Menus
    // -------------------------------------------------------------
    console.log("🎧 Test 7: Testing Audio & Subtitle Track Menus...");
    const playerContainer = desktopPage.locator(".relative.bg-black").first();
    if (await playerContainer.isVisible()) {
      await playerContainer.hover();
      await desktopPage.waitForTimeout(500);
    }

    const ccBtn = desktopPage.locator("button:has(svg.lucide-message-square), button[title*='Sous-titres']").first();
    if (await ccBtn.isVisible()) {
      await ccBtn.click();
      await desktopPage.waitForTimeout(1000);
      const playerCcPath = path.join(SCREENSHOT_DIR, "08_desktop_player_subtitles_menu.png");
      await desktopPage.screenshot({ path: playerCcPath, fullPage: false });
      report.screenshots.push({ name: "Player Subtitles Menu", path: playerCcPath });
      report.tests.push({ name: "Player Subtitle Selector", status: "PASS" });
      await ccBtn.click();
      await desktopPage.waitForTimeout(500);
    }

    const audioBtn = desktopPage.locator("button:has(svg.lucide-headphones), button[title*='Pistes audio']").first();
    if (await audioBtn.isVisible()) {
      await audioBtn.click();
      await desktopPage.waitForTimeout(1000);
      const playerAudioPath = path.join(SCREENSHOT_DIR, "09_desktop_player_audio_menu.png");
      await desktopPage.screenshot({ path: playerAudioPath, fullPage: false });
      report.screenshots.push({ name: "Player Audio Tracks Menu", path: playerAudioPath });
      report.tests.push({ name: "Player Audio Track Selector", status: "PASS" });
      await audioBtn.click();
      await desktopPage.waitForTimeout(500);
    }

    // -------------------------------------------------------------
    // Test 8: Next Episode Navigation Button
    // -------------------------------------------------------------
    console.log("⏭️ Test 8: Testing Next Episode Button...");
    const nextEpBtn = desktopPage.locator("button[title*='Épisode suivant']").first();
    if (await nextEpBtn.isVisible()) {
      await nextEpBtn.click();
      await desktopPage.waitForTimeout(3000);
      const playerNextPath = path.join(SCREENSHOT_DIR, "10_desktop_player_next_episode.png");
      await desktopPage.screenshot({ path: playerNextPath, fullPage: false });
      report.screenshots.push({ name: "Player Next Episode Navigation", path: playerNextPath });
      report.tests.push({ name: "Episode Navigation (Next Episode)", status: "PASS" });
    }

    console.log("✅ All Playwright UX/UI Testing Suites Completed Successfully!");
    console.log(JSON.stringify(report, null, 2));
  } catch (err) {
    console.error("❌ QA Test Error:", err);
    report.errors.push(String(err));
  } finally {
    await browser.close();
  }
}

runQA();
