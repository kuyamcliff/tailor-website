import { expect, test } from "@playwright/test";
import { trackErrors } from "./helpers";

test("guest sends a custom request with a reference photo", async ({ page }) => {
  const t = trackErrors(page);
  await page.goto("/custom-tailor/request");
  const main = page.locator("#main");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(main.getByText("Choose a garment.")).toBeVisible();
  await main.getByText("Traditional wear").click();
  await page.getByRole("button", { name: "Continue" }).click();
  await main.getByText("Wedding", { exact: true }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.locator("input[type=file]").setInputFiles("public/textures/irish-linen/swatch.webp");
  await expect(page.getByLabel("This photo shows")).toBeVisible();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByLabel("Your name").fill("E2E Guest");
  await page.getByLabel("Phone").fill("677000123");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Check and send" })).toBeVisible();
  await page.getByRole("button", { name: "Send request" }).click();
  await page.waitForURL(/\/requests\//);
  await expect(page.getByText(/received|thank you/i).first()).toBeVisible();
  t.assertClean();
});

test("studio design goes to a quote request", async ({ page }) => {
  const t = trackErrors(page);
  await page.goto("/studio?garment=suit");
  await expect(page.locator("canvas")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("Estimate", { exact: true })).toBeVisible({ timeout: 20_000 });
  await page.getByText("Peak lapel").click();
  await page.getByRole("button", { name: "Request a quote" }).click();
  await page.waitForURL(/custom-tailor\/request\?design=/);
  await expect(page.getByText("Your saved studio design is attached")).toBeVisible();
  t.assertClean();
});
