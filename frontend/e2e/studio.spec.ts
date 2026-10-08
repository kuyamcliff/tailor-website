import { expect, test, type Page } from "@playwright/test";
import { trackErrors } from "./helpers";

// 3D smoke tests: the scene mounts, models load, and the camera, garment, fabric and body controls all
// work without console errors. Runs on software WebGL in CI, so it checks behaviour, not frame rate.

async function openStudio(page: Page, garment = "suit", extra = "") {
  await page.goto(`/studio?garment=${garment}${extra}`);
  await expect(page.locator("canvas")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("Estimate", { exact: true })).toBeVisible({ timeout: 20_000 });
}

test("camera controls: every angle, zoom, reset, slow turn and keys", async ({ page }) => {
  const t = trackErrors(page);
  await openStudio(page);
  const angles = page.getByRole("group", { name: "View angle" });
  for (const name of [
    "Front view",
    "45 degree view",
    "Left side view",
    "135 degree view",
    "Back view",
    "Right side view",
  ]) {
    await angles.getByRole("button", { name }).click();
    await expect(angles.getByRole("button", { name })).toHaveAttribute("aria-pressed", "true");
  }
  await page.getByRole("button", { name: "Zoom in" }).click();
  await page.getByRole("button", { name: "Zoom out" }).click();
  const spin = page.getByRole("button", { name: "Turn slowly" });
  await spin.click();
  await expect(spin).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "Reset view" }).click();
  await expect(spin).toHaveAttribute("aria-pressed", "false");
  await expect(angles.getByRole("button", { name: "Front view" })).toHaveAttribute("aria-pressed", "true");

  const stage = page.getByRole("region", { name: "3D preview" });
  await stage.focus();
  await page.keyboard.press("ArrowRight");
  await expect(angles.getByRole("button", { name: "Front view" })).toHaveAttribute("aria-pressed", "false");
  await page.keyboard.press("Home");
  await expect(angles.getByRole("button", { name: "Front view" })).toHaveAttribute("aria-pressed", "true");
  t.assertClean();
});

test("measurements pin the fit estimate to the figure", async ({ page }) => {
  const t = trackErrors(page);
  await openStudio(page);
  await page.getByText("Enter mine").click();
  const values: [RegExp, string][] = [
    [/^Height/, "180"],
    [/^Neck/, "40"],
    [/^Shoulder width/, "46"],
    [/^Chest/, "104"],
    [/^Waist/, "92"],
    [/^Seat/, "104"],
    [/^Sleeve length/, "60"],
    [/^Jacket length/, "76"],
    [/^Upper arm/, "34"],
    [/^Wrist/, "18"],
    [/^Inseam/, "80"],
    [/^Thigh/, "60"],
  ];
  for (const [label, v] of values) await page.getByLabel(label).first().fill(v);
  await expect(page.getByText("Estimated good fit across measured areas")).toBeVisible({ timeout: 10_000 });
  await page.getByText("Show the fit on the figure").click();
  await expect(page.locator(".fit-pin").first()).toBeVisible();
  await expect(page.locator(".fit-pin", { hasText: "Chest" })).toContainText("Good fit");
  t.assertClean();
});

test("garment and fabric switching does not leak GPU memory", async ({ page }) => {
  test.setTimeout(300_000);
  const t = trackErrors(page);
  await openStudio(page, "suit", "&diagnostics=1");
  const diag = page.getByRole("definition").filter({ hasText: "geometries" });
  const geometries = async () => Number((await diag.innerText()).match(/(\d+) geometries/)?.[1] ?? "0");
  await expect(diag).toBeVisible({ timeout: 20_000 });
  await expect.poll(geometries, { timeout: 30_000 }).toBeGreaterThan(0);
  await page.waitForTimeout(1000);
  const baseline = await geometries();
  const garment = page.locator("#studio-garment");
  const order = ["dress", "shirt", "jacket", "gown", "suit"];
  for (let i = 0; i < 50; i++) {
    await garment.selectOption(order[i % order.length]!);
    await expect(page.locator("canvas")).toBeVisible();
    if (i % 2 === 0) {
      const fabrics = page.locator('input[name="studio-fabric"]');
      const n = await fabrics.count();
      if (n > 1) await fabrics.nth(i % n).check({ force: true });
    }
    await page.waitForTimeout(400);
  }
  await garment.selectOption("suit");
  await page.waitForTimeout(3000);
  // Two models are shown and at most four more stay cached; anything beyond that is a leak.
  const after = await geometries();
  test.info().annotations.push({ type: "geometries", description: `${baseline} before, ${after} after 50 switches` });
  expect(after).toBeLessThanOrEqual(baseline * 4);
  t.assertClean();
});

test("without WebGL the studio shows still renders and still saves", async ({ page }) => {
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement, type: string, ...rest: unknown[]) {
      if (type.startsWith("webgl")) return null;
      return (original as (...a: unknown[]) => unknown).call(this, type, ...rest);
    } as typeof original;
  });
  const t = trackErrors(page);
  await page.goto("/studio?garment=dress");
  await expect(page.getByText("Your browser cannot show the 3D preview.")).toBeVisible({ timeout: 20_000 });
  const still = page.getByRole("img", { name: /Dress preview, front view/ });
  await expect(still).toBeVisible();
  await page.getByRole("button", { name: "Back view" }).click();
  await expect(page.getByRole("img", { name: /Dress preview, back view/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Request a quote" })).toBeEnabled({ timeout: 20_000 });
  t.assertClean();
});
