import { test, expect } from "@playwright/test";

test.describe("Auth Guard - Redirect unauthenticated users", () => {
  const protectedRoutes = ["/", "/recipes", "/categories", "/settings"];

  for (const route of protectedRoutes) {
    test(`${route} redirects to /login`, async ({ page }) => {
      await page.goto(route);

      // AuthGuard should redirect to /login
      await expect(page).toHaveURL(/\/login/, { timeout: 10000 });
    });
  }
});
