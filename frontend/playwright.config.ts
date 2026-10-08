import { defineConfig, devices } from "@playwright/test";

// End-to-end tests run against a running stack (API with PAYMENTS_DEV_SIMULATOR=true and seed-dev data,
// plus this app). BASE_URL defaults to the local dev server. CHROMIUM_PATH lets CI or sandboxes use a
// preinstalled Chromium instead of downloading one.
const launchOptions = {
  executablePath: process.env.CHROMIUM_PATH || undefined,
  args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
};

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 }, launchOptions } },
    { name: "mobile", use: { ...devices["Pixel 7"], launchOptions }, testMatch: /(storefront|responsive)\.spec\.ts/ },
  ],
});
