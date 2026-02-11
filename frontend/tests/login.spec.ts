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

  test("can switch to sign up mode", async ({ page }) => {
    await page.getByRole("button", { name: "新規登録" }).click();

    await expect(page.getByText("新しいアカウントを作成")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "アカウント作成" })
    ).toBeVisible();
    // Password requirements hint
    await expect(
      page.getByText("8文字以上、大文字・小文字・数字を含む")
    ).toBeVisible();
  });

  test("can switch back to sign in from sign up", async ({ page }) => {
    await page.getByRole("button", { name: "新規登録" }).click();
    await expect(page.getByText("新しいアカウントを作成")).toBeVisible();

    await page.getByRole("button", { name: "サインイン" }).click();
    await expect(page.getByText("アカウントにサインイン")).toBeVisible();
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
