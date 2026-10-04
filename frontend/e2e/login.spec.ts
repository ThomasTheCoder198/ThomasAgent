import { expect, test } from "@playwright/test";

const email = process.env.CORE_AUTH_OWNER_EMAIL ?? "";
const password = process.env.CORE_AUTH_OWNER_PASSWORD ?? "";

test("owner signs in and lands on chat", async ({ page }) => {
  await page.goto("/chat");
  await expect(page).toHaveURL(/\/login\?next=%2Fchat/);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill(password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page).toHaveURL(/\/chat$/);
});

test("wrong password shows the localized error", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill("definitely wrong!!");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  // Scoped to the form: Next 16 also mounts its route announcer with role="alert".
  await expect(page.locator("form").getByRole("alert")).toHaveText("Email hoặc mật khẩu không đúng.");
});
