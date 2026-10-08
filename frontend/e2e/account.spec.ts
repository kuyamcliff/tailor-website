import { expect, test } from "@playwright/test";
import { owner, signIn, trackErrors } from "./helpers";

test("customer signs up and saves measurements", async ({ page }) => {
  const t = trackErrors(page);
  await page.goto("/account/sign-up?next=/account/measurements");
  await page.getByLabel("Full name").fill("E2E Account");
  await page.getByLabel("Email").fill(`e2e${Date.now()}@example.test`);
  await page.getByLabel("Password").fill("correct-horse-battery");
  await page.getByRole("button", { name: "Create account" }).click();
  await page.waitForURL(/\/account\/measurements/);
  await page.getByRole("button", { name: "New measurement profile" }).click();
  await page.getByLabel("Name", { exact: true }).fill("Me");
  await page.getByRole("button", { name: "Create profile" }).click();
  await page.getByRole("button", { name: "Add measurements" }).click();
  await page.getByLabel("Height").fill("178");
  await page.locator("#m-chest").fill("98");
  await page.getByRole("button", { name: "Save new version" }).click();
  await expect(page.getByText(/Entered by you/)).toBeVisible();
  t.assertClean();
});

test("owner drafts and sends a quote from a request", async ({ page }) => {
  const t = trackErrors(page);
  await signIn(page, owner.email, owner.password, "/owner");
  await page.goto("/owner/requests");
  const first = page.locator("table a.link").first();
  test.skip(!(await first.count()), "no requests to quote");
  await first.click();
  const draft = page.getByRole("button", { name: "Draft a quote" });
  test.skip(!(await draft.count()), "request already quoted");
  await draft.click();
  await page.waitForURL(/owner\/quotes\//);
  await page.getByLabel("Line 1 unit price").fill("95000");
  await page.getByRole("button", { name: /Send to customer/ }).click();
  await expect(page.locator("header .badge").first()).toHaveText(/Sent/);
  t.assertClean();
});
