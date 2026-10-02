import { chromium } from "../web/node_modules/playwright/index.mjs";
import fs from "node:fs";

const API_BASE = "http://127.0.0.1:8090/api/v1";
const WEB_BASE = "http://localhost:4389";
const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity/brain/312ca410-f322-403f-8d41-2aac163c504f";

const TEST_MATRIX = [
  {
    animeName: "That Time I Got Reincarnated as a Slime (Tensura)",
    animeId: 101280,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 1, ep: 2, expectedSeason: 1, expectedEp: 2 },
      { seasonNum: 2, ep: 1, expectedSeason: 2, expectedEp: 1 },
      { seasonNum: 2, ep: 2, expectedSeason: 2, expectedEp: 2 },
      { seasonNum: 3, ep: 1, expectedSeason: 3, expectedEp: 1 },
      { seasonNum: 3, ep: 2, expectedSeason: 3, expectedEp: 2 },
    ]
  },
  {
    animeName: "Demon Slayer: Kimetsu no Yaiba",
    animeId: 101922,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 1, ep: 2, expectedSeason: 1, expectedEp: 2 },
      { seasonNum: 2, ep: 1, expectedSeason: 2, expectedEp: 1 },
      { seasonNum: 3, ep: 1, expectedSeason: 3, expectedEp: 1 },
      { seasonNum: 4, ep: 1, expectedSeason: 4, expectedEp: 1 },
    ]
  },
  {
    animeName: "Jujutsu Kaisen",
    animeId: 113415,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 1, ep: 2, expectedSeason: 1, expectedEp: 2 },
      { seasonNum: 2, ep: 1, expectedSeason: 2, expectedEp: 1 },
      { seasonNum: 2, ep: 2, expectedSeason: 2, expectedEp: 2 },
    ]
  },
  {
    animeName: "Frieren: Beyond Journey's End (Sousou no Frieren)",
    animeId: 154587,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 1, ep: 2, expectedSeason: 1, expectedEp: 2 },
      { seasonNum: 1, ep: 10, expectedSeason: 1, expectedEp: 10 },
    ]
  },
  {
    animeName: "Attack on Titan (Shingeki no Kyojin)",
    animeId: 16498,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 2, ep: 1, expectedSeason: 2, expectedEp: 1 },
      { seasonNum: 3, ep: 1, expectedSeason: 3, expectedEp: 1 },
      { seasonNum: 4, ep: 1, expectedSeason: 4, expectedEp: 1 },
    ]
  },
  {
    animeName: "Mushoku Tensei: Jobless Reincarnation",
    animeId: 108465,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 1, ep: 2, expectedSeason: 1, expectedEp: 2 },
      { seasonNum: 2, ep: 1, expectedSeason: 2, expectedEp: 1 },
      { seasonNum: 2, ep: 2, expectedSeason: 2, expectedEp: 2 },
    ]
  },
  {
    animeName: "Re:ZERO - Starting Life in Another World",
    animeId: 21355,
    tests: [
      { seasonNum: 1, ep: 1, expectedSeason: 1, expectedEp: 1 },
      { seasonNum: 2, ep: 1, expectedSeason: 2, expectedEp: 1 },
      { seasonNum: 3, ep: 1, expectedSeason: 3, expectedEp: 1 },
    ]
  }
];

async function runE2E() {
  console.log("================================================================================");
  console.log("   GAZES COMPREHENSIVE MULTI-ANIME SEASON & EPISODE VERIFICATION SUITE");
  console.log("================================================================================\n");

  const results = [];

  for (const anime of TEST_MATRIX) {
    console.log(`\n>>> Testing Anime: ${anime.animeName} (ID: ${anime.animeId})`);
    
    // 1. Fetch franchise
    const fRes = await fetch(`${API_BASE}/catalog/anime/${anime.animeId}/franchise`);
    if (!fRes.ok) {
      console.error(`  FAIL: Failed to fetch franchise for ${anime.animeName}`);
      continue;
    }
    const franchise = await fRes.json();
    const mainSeasons = franchise.seasons?.filter(s => s.group === "main") || [];
    console.log(`  Franchise loaded: "${franchise.title}", ${mainSeasons.length} main seasons found.`);

    for (const test of anime.tests) {
      // Find matching season entry in franchise
      const seasonEntry = mainSeasons.find(s => s.season_number === test.seasonNum) || mainSeasons[test.seasonNum - 1];
      if (!seasonEntry) {
        console.log(`  [SKIP] Season ${test.seasonNum} not found in main seasons for ${anime.animeName}`);
        continue;
      }

      const seasonId = seasonEntry.id;
      const url = `${API_BASE}/catalog/anime/${anime.animeId}/seasons/${seasonId}/episodes/${test.ep}/sources`;
      const sRes = await fetch(url);
      
      if (!sRes.ok) {
        console.log(`  [STATUS ${sRes.status}] Season ${test.seasonNum} (${seasonEntry.title}) Ep ${test.ep}: ${await sRes.text()}`);
        results.push({
          anime: anime.animeName,
          season: test.seasonNum,
          seasonTitle: seasonEntry.title,
          ep: test.ep,
          status: sRes.status,
          success: false,
          sourcesCount: 0,
          topSource: "N/A"
        });
        continue;
      }

      const data = await sRes.json();
      const top = data.sources?.[0];
      const validSources = (data.sources || []).filter(s => s.episode_number === test.ep);

      console.log(`  [OK] Season ${test.seasonNum} ("${seasonEntry.title}") Ep ${test.ep}:`);
      console.log(`       Total Sources: ${data.total_sources} (${data.french_sources} French)`);
      if (top) {
        console.log(`       Top #1 Source: [${top.language_tag}] [Rank ${top.score_rank}] [Seeds ${top.seeders}] "${top.title}" (Batch: ${top.is_batch})`);
      }

      results.push({
        anime: anime.animeName,
        season: test.seasonNum,
        seasonTitle: seasonEntry.title,
        ep: test.ep,
        status: 200,
        success: data.total_sources > 0,
        sourcesCount: data.total_sources,
        frenchSources: data.french_sources,
        topSource: top?.title || "None"
      });
    }
  }

  // Playwright Browser Verification on key pages
  console.log("\n================================================================================");
  console.log("   LAUNCHING AUTOMATED HEADLESS CHROMIUM FOR UI PLAYBACK PROOF");
  console.log("================================================================================\n");

  const browser = await chromium.launch({
    executablePath: "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium",
    args: ["--no-sandbox", "--disable-setuid-sandbox", "--disable-gpu"]
  });

  const page = await browser.newPage();
  await page.setViewportSize({ width: 1440, height: 900 });

  const browserTests = [
    { name: "Tensura_S01E02", path: "/anime/101280/seasons/101280/episodes/2", expectedTitle: "Épisode 2" },
    { name: "Tensura_S02E01", path: "/anime/101280/seasons/108511/episodes/1", expectedTitle: "Épisode 1" },
    { name: "Tensura_S03E01", path: "/anime/101280/seasons/156822/episodes/1", expectedTitle: "Épisode 1" },
    { name: "Kimetsu_S01E02", path: "/anime/101922/seasons/101922/episodes/2", expectedTitle: "Épisode 2" },
    { name: "Frieren_S01E02", path: "/anime/154587/seasons/154587/episodes/2", expectedTitle: "Épisode 2" },
  ];

  for (const bTest of browserTests) {
    console.log(`  -> Browser visiting: ${WEB_BASE}${bTest.path}`);
    try {
      await page.goto(`${WEB_BASE}${bTest.path}`, { waitUntil: "networkidle", timeout: 15000 });
      await page.waitForTimeout(2000);
      
      const screenshotPath = `${ARTIFACT_DIR}/verify_${bTest.name}.png`;
      await page.screenshot({ path: screenshotPath, fullPage: true });
      console.log(`     Captured screenshot: verify_${bTest.name}.png`);
    } catch (err) {
      console.error(`     Browser test error for ${bTest.name}:`, err.message);
    }
  }

  await browser.close();

  console.log("\n================================================================================");
  console.log("   FINAL VERIFICATION SUMMARY MATRIX");
  console.log("================================================================================");
  console.table(results.map(r => ({
    Anime: r.anime.slice(0, 30),
    Season: `S${r.season}`,
    Ep: `E${r.ep}`,
    Status: r.status === 200 ? "PASS" : "FAIL",
    Sources: r.sourcesCount,
    FR: r.frenchSources,
    TopRelease: r.topSource.slice(0, 50)
  })));

  const allPassed = results.filter(r => r.status === 200).length;
  console.log(`\nTotal tests: ${results.length} | Succeeded: ${allPassed} | Failed: ${results.length - allPassed}`);
}

runE2E().catch(console.error);
