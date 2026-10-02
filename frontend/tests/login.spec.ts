import { test, expect } from "@playwright/test";

test.describe("Login Page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/login");
  });

  test("displays sign in form by default", async ({ page }) => {
    await expect(page.locator("h1")).toContainText("レシピ保存アプリ");
    await expect(page.getByText("アカウントにサインイン")).toBeVisible();

    // Form fields (label and input are not associated via for/id, use text + role)
    await expect(page.getByText("メールアドレス")).toBeVisible();
    await expect(page.getByText("パスワード")).toBeVisible();
    await expect(page.locator('input[type="email"]')).toBeVisible();
    await expect(page.locator('input[type="password"]')).toBeVisible();
    await expect(
      page.getByRole("button", { name: "サインイン" })
    ).toBeVisible();
  });

  // 自分専用のアプリなので新規登録はできない
  test("does not offer sign up", async ({ page }) => {
    await expect(page.getByText("アカウントにサインイン")).toBeVisible();
    await expect(page.getByRole("button", { name: "新規登録" })).toHaveCount(0);
    await expect(page.getByText("新しいアカウントを作成")).toHaveCount(0);
  });

  test("sign in form requires email and password", async ({ page }) => {
    const submitButton = page.getByRole("button", { name: "サインイン" });

    // Try submitting empty form - HTML5 validation should prevent
    await submitButton.click();

    // Should stay on login page
    await expect(page).toHaveURL(/\/login/);
  });

  test("does not show bottom navigation", async ({ page }) => {
    // BottomNav is hidden on /login
    await expect(page.locator("nav")).toBeHidden();
  });
});
