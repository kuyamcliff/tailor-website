#!/usr/bin/env node
// Renders the home page studio preview frames (front, 45, side, back) from the running fitting studio,
// so the preview always shows the studio's real model. Requires the app running on BASE_URL.
// Usage: BASE_URL=http://localhost:3000 node scripts/render-previews.mjs
import { mkdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";

const base = process.env.BASE_URL ?? "http://localhost:3000";
const out = join(dirname(fileURLToPath(import.meta.url)), "..", "public", "3d", "renders");
mkdirSync(out, { recursive: true });
const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH || undefined,
  args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
});
const page = await browser.newPage({ viewport: { width: 1600, height: 1100 }, deviceScaleFactor: 1 });
await page.goto(`${base}/studio?garment=suit`, { waitUntil: "networkidle" });
await page.waitForSelector("canvas");
// Capture only the stage: hide the site chrome and the studio panel and controls.
await page.addStyleTag({ content: "header, footer, aside, [role=group], .skip-link { visibility: hidden !important; } [class*=standin], [class*=controls] { visibility: hidden !important; }" });
await page.waitForTimeout(3000);
for (const [key, label] of [["front", "Front"], ["45", "45°"], ["side", "Side"], ["back", "Back"]]) {
  await page.locator(`[aria-label="View angle"] button`).filter({ hasText: new RegExp(`^${label}$`) }).dispatchEvent("click");
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(1500);
  const box = await page.locator("canvas").boundingBox();
  const w = Math.min(box.width, box.height * 0.8);
  await page.screenshot({ path: join(out, `suit-${key}.png`), clip: { x: box.x + (box.width - w) / 2, y: box.y + 40, width: w, height: box.height - 110 } });
  console.log("rendered", key);
}
await browser.close();
