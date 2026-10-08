import { expect, test } from "@playwright/test";
import { owner, signIn, trackErrors } from "./helpers";

// Owner screens for things that used to need seed data: new garment types with their own options,
// choices and measurements, product categories, and booking an appointment for a customer.

test("owner adds a garment with options, measurements and a fit rule", async ({ page }) => {
  const t = trackErrors(page);
  const name = `Kaftan ${Date.now().toString(36)}`;
  await signIn(page, owner.email, owner.password, "/owner");
  await page.goto("/owner/garments");
  await page.getByRole("button", { name: "Add a garment" }).click();
  const form = page.getByRole("form", { name: "New garment" });
  await form.getByLabel("Name").fill(name);
  await form.getByLabel("Category").fill("traditional");
  await form.getByRole("button", { name: "Add garment" }).click();
  await page.waitForURL(/\/owner\/garments\/kaftan-/);
  await expect(page.getByRole("heading", { name })).toBeVisible();

  await page.getByRole("button", { name: "Add an option" }).click();
  const opt = page.getByRole("form", { name: "New option" });
  await opt.getByLabel("Name").fill("Neckline");
  await opt.getByLabel("Section in the studio").fill("Top");
  await opt.getByRole("button", { name: "Add option" }).click();
  await expect(page.getByText("Neckline added.")).toBeVisible();
  await page.getByText("Neckline", { exact: true }).click();
  await page.getByLabel("New Neckline choice").fill("Round");
  await page.getByRole("button", { name: "Add choice" }).click();
  await expect(page.getByText("Round added.")).toBeVisible();
  await expect(page.getByLabel("Neckline choice 1 name")).toHaveValue("Round");

  await page.getByRole("button", { name: "New measurement" }).click();
  const mf = page.getByRole("form", { name: "New measurement" });
  const label = `Robe length ${Date.now().toString(36)}`;
  await mf.getByLabel("Name").fill(label);
  await mf.getByLabel("Smallest (cm)").fill("80");
  await mf.getByLabel("Largest (cm)").fill("170");
  await mf.getByRole("button", { name: "Add measurement" }).click();
  await expect(page.getByText(`${label} added.`)).toBeVisible();
  await page.getByLabel(`Ask for ${label}`).check();
  await page.getByLabel("Ask for Chest", { exact: true }).check();
  await page.getByLabel("Chest required", { exact: true }).check();
  await page.getByRole("button", { name: "Save measurements" }).click();
  await expect(page.getByText("Measurements saved.")).toBeVisible();

  await page.getByLabel("Measurement for a new fit rule").selectOption({ label: "Chest" });
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await page.getByRole("button", { name: "Save fit rules" }).click();
  await expect(page.getByText("Fit rules saved.")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("button", { name: "Remove Chest" })).toBeVisible();
  t.assertClean();
});

test("owner adds a product category", async ({ page }) => {
  const t = trackErrors(page);
  const name = `Capes ${Date.now().toString(36)}`;
  await signIn(page, owner.email, owner.password, "/owner");
  await page.goto("/owner/products");
  await page.getByLabel("New category name").fill(name);
  await page.getByRole("button", { name: "Add category" }).click();
  await expect(page.getByText(`${name} added.`)).toBeVisible();
  await expect(page.getByLabel(`${name} name`)).toBeVisible();
  t.assertClean();
});

test("owner books an appointment for a customer", async ({ page }) => {
  const t = trackErrors(page);
  await signIn(page, owner.email, owner.password, "/owner");
  await page.goto("/owner/customers");
  const first = page.locator("table tbody tr a").first();
  const customerName = (await first.innerText()).trim();
  await first.click();
  await page.getByRole("link", { name: "Book an appointment" }).click();
  await page.waitForURL(/\/owner\/appointments\?book=1/);
  const form = page.locator("section[aria-labelledby=book-h]");
  await expect(form.getByText(customerName).first()).toBeVisible();
  await form.getByRole("combobox", { name: "Type", exact: true }).selectOption("measuring");
  await form.getByLabel("Choose another time").check();
  // A random far-off early morning, so reruns never collide with an earlier booking.
  const day = new Date(Date.now() + 86_400_000 * (30 + Math.floor(Math.random() * 600))).toISOString().slice(0, 10);
  await form.getByLabel("Day").fill(day);
  await form.getByLabel(/^Time/).fill("07:15");
  await form.getByText("Book even outside opening hours").click();
  await form.getByLabel("Internal note (staff only)").fill("Booked by phone");
  await form.getByRole("button", { name: "Book appointment" }).click();
  await expect(page.getByText(`Booked for ${customerName}.`)).toBeVisible();
  t.assertClean();
});
