import { test } from "@playwright/test";
import { noHorizontalOverflow } from "./helpers";

// Saves full-page screenshots at common widths for visual review and fails on sideways scrolling.
const widths = [320, 375, 768, 1024, 1280, 1440, 1920];
const pages = ["/", "/shop", "/custom-tailor", "/custom-tailor/request", "/studio", "/appointments", "/contact"];

for (const path of pages) {
  test(`layout ${path}`, async ({ page }, info) => {
    test.skip(info.project.name !== "desktop", "widths are set explicitly");
    for (const w of widths) {
      await page.setViewportSize({ width: w, height: 900 });
      await page.goto(path, { waitUntil: "networkidle" });
      await noHorizontalOverflow(page);
      await page.screenshot({ path: info.outputPath(`${path === "/" ? "home" : path.slice(1).replaceAll("/", "_")}-${w}.png`), fullPage: true });
    }
  });
}
