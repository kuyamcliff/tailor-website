#!/usr/bin/env node
// Renders still angles (front, 45, left, 135, back, right) of every studio garment from the running
// fitting studio, so the home page preview and the no-WebGL fallback show the studio's real model with
// its default options. Frames are read straight from the WebGL canvas, so no page chrome is captured.
// The render URLs (with a content hash for the immutable /3d/ cache) are written into the asset
// manifest's supportedOptions.renders; build-standin-assets.mjs keeps them when it regenerates.
// Requires the app and API running: BASE_URL=http://localhost:3000 node scripts/render-previews.mjs
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";

const base = process.env.BASE_URL ?? "http://localhost:3000";
const dir3d = join(dirname(fileURLToPath(import.meta.url)), "..", "public", "3d");
const out = join(dir3d, "renders");
mkdirSync(out, { recursive: true });
const views = [
  ["front", "Front view"],
  ["45", "45 degree view"],
  ["side", "Left side view"],
  ["135", "135 degree view"],
  ["back", "Back view"],
  ["right", "Right side view"],
];

const garments = (await (await fetch(`${base}/api/v1/garments`)).json()).filter((g) => g.studioEnabled);
const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH || undefined,
  args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
});
const page = await browser.newPage({ viewport: { width: 1600, height: 1100 }, deviceScaleFactor: 1 });
const rendersByAsset = {};
for (const g of garments) {
  const studio = await (await fetch(`${base}/api/v1/garments/${g.key}/studio`)).json();
  if (!studio.asset) continue;
  await page.goto(`${base}/studio?garment=${g.key}`, { waitUntil: "networkidle" });
  await page.waitForSelector("canvas");
  await page.getByLabel("Detail").selectOption("ultra");
  await page.waitForTimeout(4000);
  const renders = {};
  for (const [key, name] of views) {
    await page.getByRole("button", { name, exact: true }).click();
    await page.waitForTimeout(Number(process.env.SETTLE_MS ?? 5000));
    const dataUrl = await page.evaluate(() => {
      const c = document.querySelector("canvas");
      const w = Math.min(c.width, Math.round(c.height * 0.8));
      const h = c.height - Math.round(c.height * 0.12);
      const crop = document.createElement("canvas");
      crop.width = w;
      crop.height = h;
      crop.getContext("2d").drawImage(c, (c.width - w) / 2, Math.round(c.height * 0.04), w, h, 0, 0, w, h);
      return crop.toDataURL("image/webp", 0.86);
    });
    const buf = Buffer.from(dataUrl.split(",")[1], "base64");
    const file = `${g.key}-${key}.webp`;
    writeFileSync(join(out, file), buf);
    renders[key] = `/3d/renders/${file}?v=${createHash("sha256").update(buf).digest("hex").slice(0, 12)}`;
    console.log("rendered", file, `${(buf.length / 1024).toFixed(0)} KB`);
  }
  rendersByAsset[studio.asset.assetKey] = renders;
}
await browser.close();

const manifestPath = join(dir3d, "manifest.json");
const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
for (const a of manifest.assets)
  if (rendersByAsset[a.assetKey]) a.supportedOptions.renders = rendersByAsset[a.assetKey];
writeFileSync(manifestPath, JSON.stringify(manifest, null, 2) + "\n");
console.log("manifest updated");
