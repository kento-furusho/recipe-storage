import { test, expect, Page } from "@playwright/test";

/**
 * Mock Cognito auth so the settings page renders instead of redirecting to /login.
 *
 * Amplify v6 calls the Cognito IDP endpoint via fetch.  We intercept that
 * request and return fake tokens / user info so AuthProvider sees a logged-in
 * user.
 */
async function mockAuth(page: Page) {
  await page.route("https://cognito-idp.*.amazonaws.com/", async (route) => {
    const body = route.request().postData() || "";

    // GetUser / GetSession / GetId requests -- return a fake authenticated user
    const fakeIdToken = [
      btoa(JSON.stringify({ alg: "none", typ: "JWT" })),
      btoa(
        JSON.stringify({
          sub: "test-user-id",
          email: "test@example.com",
          email_verified: true,
          iss: "https://cognito-idp.ap-northeast-1.amazonaws.com/ap-northeast-1_test",
          aud: "test-client-id",
          exp: Math.floor(Date.now() / 1000) + 3600,
          iat: Math.floor(Date.now() / 1000),
        })
      ),
      "fake-signature",
    ].join(".");

    const fakeAccessToken = [
      btoa(JSON.stringify({ alg: "none", typ: "JWT" })),
      btoa(
        JSON.stringify({
          sub: "test-user-id",
          iss: "https://cognito-idp.ap-northeast-1.amazonaws.com/ap-northeast-1_test",
          exp: Math.floor(Date.now() / 1000) + 3600,
          iat: Math.floor(Date.now() / 1000),
          client_id: "test-client-id",
          token_use: "access",
        })
      ),
      "fake-signature",
    ].join(".");

    if (body.includes("InitiateAuth") || body.includes("GetUser")) {
      await route.fulfill({
        status: 200,
        contentType: "application/x-amz-json-1.1",
        body: JSON.stringify({
          AuthenticationResult: {
            IdToken: fakeIdToken,
            AccessToken: fakeAccessToken,
            RefreshToken: "fake-refresh-token",
            ExpiresIn: 3600,
            TokenType: "Bearer",
          },
          UserAttributes: [
            { Name: "sub", Value: "test-user-id" },
            { Name: "email", Value: "test@example.com" },
            { Name: "email_verified", Value: "true" },
          ],
          Username: "test-user-id",
        }),
      });
    } else if (body.includes("ChangePassword")) {
      await route.fulfill({
        status: 200,
        contentType: "application/x-amz-json-1.1",
        body: JSON.stringify({}),
      });
    } else if (body.includes("UpdateUserAttributes")) {
      await route.fulfill({
        status: 200,
        contentType: "application/x-amz-json-1.1",
        body: JSON.stringify({
          CodeDeliveryDetailsList: [
            {
              AttributeName: "email",
              DeliveryMedium: "EMAIL",
              Destination: "t***@e***",
            },
          ],
        }),
      });
    } else if (body.includes("VerifyUserAttribute")) {
      await route.fulfill({
        status: 200,
        contentType: "application/x-amz-json-1.1",
        body: JSON.stringify({}),
      });
    } else if (body.includes("GlobalSignOut") || body.includes("RevokeToken")) {
      await route.fulfill({
        status: 200,
        contentType: "application/x-amz-json-1.1",
        body: JSON.stringify({}),
      });
    } else {
      // Default: return success for any unhandled Cognito call
      await route.fulfill({
        status: 200,
        contentType: "application/x-amz-json-1.1",
        body: JSON.stringify({}),
      });
    }
  });

  // Amplify stores tokens in localStorage; seed them so getCurrentUser succeeds
  await page.addInitScript(() => {
    const fakeIdPayload = {
      sub: "test-user-id",
      email: "test@example.com",
      email_verified: true,
      iss: "https://cognito-idp.ap-northeast-1.amazonaws.com/ap-northeast-1_test",
      aud: "test-client-id",
      exp: Math.floor(Date.now() / 1000) + 3600,
      iat: Math.floor(Date.now() / 1000),
    };
    const fakeAccessPayload = {
      sub: "test-user-id",
      iss: "https://cognito-idp.ap-northeast-1.amazonaws.com/ap-northeast-1_test",
      exp: Math.floor(Date.now() / 1000) + 3600,
      iat: Math.floor(Date.now() / 1000),
      client_id: "test-client-id",
      token_use: "access",
    };
    const encode = (obj: Record<string, unknown>) => btoa(JSON.stringify(obj));
    const idToken = [
      encode({ alg: "none", typ: "JWT" }),
      encode(fakeIdPayload),
      "fake-signature",
    ].join(".");
    const accessToken = [
      encode({ alg: "none", typ: "JWT" }),
      encode(fakeAccessPayload),
      "fake-signature",
    ].join(".");

    // Amplify v6 uses localStorage keys based on the user pool / client config.
    // We set the key that Amplify's CognitoUserPool looks for.
    const lastAuthUser = "test-user-id";
    const keyPrefix = "CognitoIdentityServiceProvider.test-client-id";
    localStorage.setItem(`${keyPrefix}.LastAuthUser`, lastAuthUser);
    localStorage.setItem(
      `${keyPrefix}.${lastAuthUser}.idToken`,
      idToken
    );
    localStorage.setItem(
      `${keyPrefix}.${lastAuthUser}.accessToken`,
      accessToken
    );
    localStorage.setItem(
      `${keyPrefix}.${lastAuthUser}.refreshToken`,
      "fake-refresh-token"
    );
  });
}

test.describe("Settings Page", () => {
  test.describe("Page rendering (authenticated)", () => {
    test.beforeEach(async ({ page }) => {
      await mockAuth(page);
      await page.goto("/settings");
    });

    test("displays all sections", async ({ page }) => {
      // Account info section
      await expect(page.getByText("アカウント情報")).toBeVisible();

      // Email change section
      await expect(page.getByText("メールアドレス変更")).toBeVisible();

      // Password change section
      await expect(page.getByText("パスワード変更")).toBeVisible();

      // Sign out section
      await expect(page.getByText("サインアウト")).toBeVisible();
    });

    test("shows user email in account info", async ({ page }) => {
      await expect(page.getByText("アカウント情報")).toBeVisible();
      await expect(page.getByText("test@example.com")).toBeVisible();
    });

    test("shows sign-out button", async ({ page }) => {
      const signOutButton = page.getByRole("button", { name: "サインアウト" });
      await expect(signOutButton).toBeVisible();
    });
  });

  test.describe("Password change form", () => {
    test.beforeEach(async ({ page }) => {
      await mockAuth(page);
      await page.goto("/settings");
      // Wait for the page to fully render
      await expect(page.getByText("パスワード変更")).toBeVisible();
    });

    test("displays three password fields and submit button", async ({
      page,
    }) => {
      await expect(
        page.getByPlaceholder("現在のパスワード")
      ).toBeVisible();
      await expect(
        page.getByPlaceholder("新しいパスワード", { exact: true })
      ).toBeVisible();
      await expect(
        page.getByPlaceholder("新しいパスワード（確認）")
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "パスワードを変更" })
      ).toBeVisible();
    });

    test("shows error when new password is less than 8 characters", async ({
      page,
    }) => {
      await page.getByPlaceholder("現在のパスワード").fill("oldpass123");
      await page
        .getByPlaceholder("新しいパスワード", { exact: true })
        .fill("short");
      await page.getByPlaceholder("新しいパスワード（確認）").fill("short");

      await page.getByRole("button", { name: "パスワードを変更" }).click();

      await expect(
        page.getByText("パスワードは8文字以上にしてください")
      ).toBeVisible();
    });

    test("shows error when new passwords do not match", async ({ page }) => {
      await page.getByPlaceholder("現在のパスワード").fill("oldpass123");
      await page
        .getByPlaceholder("新しいパスワード", { exact: true })
        .fill("newpassword1");
      await page
        .getByPlaceholder("新しいパスワード（確認）")
        .fill("newpassword2");

      await page.getByRole("button", { name: "パスワードを変更" }).click();

      await expect(
        page.getByText("新しいパスワードが一致しません")
      ).toBeVisible();
    });

    test("validates minimum length before mismatch", async ({ page }) => {
      // When password is both too short AND mismatched, the length error
      // should appear first (it is checked first in the handler).
      await page.getByPlaceholder("現在のパスワード").fill("oldpass123");
      await page
        .getByPlaceholder("新しいパスワード", { exact: true })
        .fill("short1");
      await page
        .getByPlaceholder("新しいパスワード（確認）")
        .fill("short2");

      await page.getByRole("button", { name: "パスワードを変更" }).click();

      await expect(
        page.getByText("パスワードは8文字以上にしてください")
      ).toBeVisible();
    });

    test("requires all fields via HTML5 validation", async ({ page }) => {
      // Submit without filling anything - HTML5 required should prevent
      await page.getByRole("button", { name: "パスワードを変更" }).click();

      // Should stay on settings page (no JS validation errors shown because
      // the browser's built-in validation stops submission)
      await expect(page).toHaveURL(/\/settings/);
    });
  });

  test.describe("Email change form", () => {
    test.beforeEach(async ({ page }) => {
      await mockAuth(page);
      await page.goto("/settings");
      await expect(page.getByText("メールアドレス変更")).toBeVisible();
    });

    test("displays email input and submit button in input step", async ({
      page,
    }) => {
      await expect(
        page.getByPlaceholder("新しいメールアドレス")
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "確認コードを送信" })
      ).toBeVisible();
    });

    test("shows error when entering the same email as current", async ({
      page,
    }) => {
      await page
        .getByPlaceholder("新しいメールアドレス")
        .fill("test@example.com");

      await page.getByRole("button", { name: "確認コードを送信" }).click();

      await expect(
        page.getByText("現在のメールアドレスと同じです")
      ).toBeVisible();
    });

    test("can type a new email address", async ({ page }) => {
      const emailInput = page.getByPlaceholder("新しいメールアドレス");
      await emailInput.fill("newemail@example.com");
      await expect(emailInput).toHaveValue("newemail@example.com");
    });

    test("email input has type=email for browser validation", async ({
      page,
    }) => {
      const emailInput = page.getByPlaceholder("新しいメールアドレス");
      await expect(emailInput).toHaveAttribute("type", "email");
    });
  });

  test.describe("Redirect for unauthenticated users", () => {
    test("redirects to /login when not authenticated", async ({ page }) => {
      // No auth mock -- AuthGuard should redirect
      await page.goto("/settings");
      await expect(page).toHaveURL(/\/login/, { timeout: 10000 });
    });
  });
});
