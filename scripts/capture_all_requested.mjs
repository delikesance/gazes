import { chromium } from "../web/node_modules/playwright/index.mjs";

const WEB_BASE = "http://localhost:4389";
const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity/brain/312ca410-f322-403f-8d41-2aac163c504f";
const CHROMIUM_EXEC = "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium";

const SCREENS = [
  { name: "01_tensura_s01e02", path: "/anime/101280/seasons/101280/episodes/2", desc: "Tensura Season 1 Episode 2" },
  { name: "02_tensura_s02e01", path: "/anime/101280/seasons/108511/episodes/1", desc: "Tensura Season 2 Episode 1" },
  { name: "03_tensura_s03e01", path: "/anime/101280/seasons/156822/episodes/1", desc: "Tensura Season 3 Episode 1" },
  { name: "04_demon_slayer_s01e02", path: "/anime/101922/seasons/101922/episodes/2", desc: "Demon Slayer Season 1 Episode 2" },
  { name: "05_frieren_s01e02", path: "/anime/154587/seasons/154587/episodes/2", desc: "Frieren Season 1 Episode 2" },
  { name: "06_jujutsu_kaisen_s02e01", path: "/anime/113415/seasons/145064/episodes/1", desc: "Jujutsu Kaisen Season 2 Episode 1" },
  { name: "07_attack_on_titan_s04e01", path: "/anime/16498/seasons/110277/episodes/1", desc: "Attack on Titan Season 4 Episode 1" },
];

async function captureScreens() {
  console.log("Launching Chromium to capture verified anime playback screens...");
  const browser = await chromium.launch({
    executablePath: CHROMIUM_EXEC,
    headless: true,
    args: ["--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage", "--disable-gpu"]
  });

  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

  // Listen to console logs
  page.on("console", msg => {
    if (msg.text().includes("[Gazes:")) {
      console.log(`  [BROWSER] ${msg.text()}`);
    }
  });

  for (const s of SCREENS) {
    console.log(`\nNavigating to ${s.desc} (${s.path})...`);
    try {
      await page.goto(`${WEB_BASE}${s.path}`, { waitUntil: "domcontentloaded", timeout: 20000 });
      // Wait for UI to render player and heading
      await page.waitForTimeout(3500);
      const filePath = `${ARTIFACT_DIR}/${s.name}.png`;
      await page.screenshot({ path: filePath, fullPage: false });
      console.log(`  -> Saved screenshot: ${filePath}`);
    } catch (err) {
      console.error(`  -> Failed for ${s.name}:`, err.message);
    }
  }

  await browser.close();
  console.log("\nAll requested anime screens captured successfully!");
}

captureScreens().catch(console.error);
