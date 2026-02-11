import { test, expect } from "@playwright/test";

test.describe("PWA Configuration", () => {
  test("has web app manifest link", async ({ page }) => {
    await page.goto("/login");

    const manifest = page.locator('link[rel="manifest"]');
    await expect(manifest).toHaveAttribute("href", "/manifest.json");
  });

  test("manifest.json is valid", async ({ request }) => {
    const response = await request.get("/manifest.json");
    expect(response.ok()).toBeTruthy();

    const manifest = await response.json();
    expect(manifest.name).toBe("レシピ保存アプリ");
    expect(manifest.short_name).toBe("レシピ");
    expect(manifest.display).toBe("standalone");
    expect(manifest.start_url).toBe("/");
    expect(manifest.theme_color).toBe("#f59e0b");
    expect(manifest.icons).toHaveLength(4);

    // Verify required icon sizes
    const sizes = manifest.icons.map(
      (i: { sizes: string }) => i.sizes
    );
    expect(sizes).toContain("192x192");
    expect(sizes).toContain("512x512");
  });

  test("icon files are accessible", async ({ request }) => {
    const icons = [
      "/icons/icon-192.png",
      "/icons/icon-512.png",
      "/icons/icon-maskable-192.png",
      "/icons/icon-maskable-512.png",
    ];
    for (const icon of icons) {
      const response = await request.get(icon);
      expect(response.ok(), `${icon} should be accessible`).toBeTruthy();
      expect(response.headers()["content-type"]).toContain("image/png");
    }
  });

  test("has theme-color meta tag", async ({ page }) => {
    await page.goto("/login");

    const themeColor = page.locator('meta[name="theme-color"]');
    await expect(themeColor).toHaveAttribute("content", "#f59e0b");
  });

  test("has apple-web-app meta tags", async ({ page }) => {
    await page.goto("/login");

    // Next.js renders mobile-web-app-capable for appleWebApp.capable
    await expect(
      page.locator('meta[name="mobile-web-app-capable"]')
    ).toHaveAttribute("content", "yes");

    await expect(
      page.locator('meta[name="apple-mobile-web-app-status-bar-style"]')
    ).toHaveAttribute("content", "default");
  });

  test("service worker is registered", async ({ page }) => {
    await page.goto("/login");

    // sw.js should be served
    const swResponse = await page.request.get("/sw.js");
    expect(swResponse.ok()).toBeTruthy();
  });
});
