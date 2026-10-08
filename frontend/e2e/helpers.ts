import { expect, type Page } from "@playwright/test";

// trackErrors fails a test on uncaught page errors, hydration mismatches and failed requests.
export function trackErrors(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() === "error" && !/Download the React DevTools/.test(m.text())) errors.push(`console: ${m.text()} (${m.location().url})`);
  });
  page.on("response", (r) => {
    if (r.status() >= 500 || (r.status() === 404 && !r.url().includes("/api/"))) errors.push(`${r.status()} ${r.url()}`);
  });
  return {
    assertClean: () => expect(errors, errors.join("\n")).toEqual([]),
  };
}

export async function signIn(page: Page, email: string, password: string, next = "/account") {
  await page.goto(`/account/sign-in?next=${encodeURIComponent(next)}`);
  await page.getByLabel("Email or phone").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL((u) => u.pathname.startsWith(next));
}

export const owner = { email: process.env.E2E_OWNER_EMAIL ?? "owner@atelier.test", password: process.env.E2E_OWNER_PASSWORD ?? "atelier-owner-dev" };

export async function noHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow, "page scrolls sideways").toBeLessThanOrEqual(0);
}
