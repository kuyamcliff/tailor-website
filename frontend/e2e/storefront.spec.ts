import { expect, test } from "@playwright/test";
import { noHorizontalOverflow, trackErrors } from "./helpers";

test("home, shop and product pages load cleanly", async ({ page }) => {
  const t = trackErrors(page);
  for (const path of [
    "/",
    "/shop",
    "/custom-tailor",
    "/our-work",
    "/about",
    "/contact",
    "/appointments",
    "/policies/privacy",
    "/policies/cookies",
  ]) {
    await page.goto(path, { waitUntil: "networkidle" });
    await expect(page.locator("h1").first()).toBeVisible();
    await noHorizontalOverflow(page);
  }
  t.assertClean();
});

test("guest checkout and a simulated MTN MoMo payment confirmed by the server", async ({ page }) => {
  const t = trackErrors(page);
  // Use the first product that is in stock (earlier runs may have used up a size).
  await page.goto("/shop");
  const links = await page
    .locator("article a[href^='/shop/']")
    .evaluateAll((as) => [...new Set(as.map((a) => a.getAttribute("href")))]);
  let added = false;
  for (const href of links.slice(0, 8)) {
    await page.goto(href!);
    const sizes = page.getByRole("group", { name: /size/i }).locator("button:not([disabled])");
    if (await page.getByRole("group", { name: /size/i }).count()) {
      if (!(await sizes.count())) continue;
      await sizes.first().click();
    }
    const add = page.getByRole("button", { name: "Add to bag" });
    if (!(await add.isEnabled())) continue;
    await add.click();
    added = true;
    break;
  }
  expect(added, "no product in stock").toBe(true);
  await expect(page.getByRole("button", { name: /^Bag, [1-9]/ }).first()).toBeAttached();
  await page.goto("/checkout");
  await page.getByLabel("Full name").fill("E2E Customer");
  await page.getByLabel("Phone", { exact: true }).fill("677000001");
  await page.getByRole("button", { name: "Place order" }).click();
  await page.waitForURL(/\/orders\//);
  await page.getByLabel("Mobile Money number").fill("677000001");
  await page.getByRole("button", { name: /^Pay/ }).click();
  // Suffix 0001 is the simulator's success scenario; the order only shows paid after server verification.
  await expect(page.getByText("Payment received")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText(/test payment|no money/i).first()).toBeVisible();
  t.assertClean();
});
