import { chromium } from "../web/node_modules/playwright/index.mjs";
import path from "path";

const ARTIFACT_DIR = "/home/nixos/.gemini/antigravity-cli/brain/2a7fea93-6344-4507-be95-97efb8c62c61";
const CHROMIUM_PATH = "/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium";

async function debugNetwork() {
  const browser = await chromium.launch({
    executablePath: CHROMIUM_PATH,
    headless: true,
    args: ["--no-sandbox", "--disable-setuid-sandbox"],
  });

  const page = await browser.newPage();

  page.on("request", (req) => console.log("➡️ [REQUEST]", req.method(), req.url()));
  page.on("response", async (res) => {
    let body = "";
    try {
      if (res.url().includes("/api/")) {
        body = (await res.text()).slice(0, 200);
      }
    } catch {}
    console.log("⬅️ [RESPONSE]", res.status(), res.url(), body);
  });
  page.on("requestfailed", (req) => console.error("❌ [FAILED REQUEST]", req.url(), req.failure()?.errorText));
  page.on("console", (msg) => console.log("💬 [CONSOLE]", msg.type(), msg.text()));

  console.log("Navigating to http://127.0.0.1:4389...");
  await page.goto("http://127.0.0.1:4389");
  await page.waitForTimeout(5000);

  await browser.close();
}

debugNetwork();
