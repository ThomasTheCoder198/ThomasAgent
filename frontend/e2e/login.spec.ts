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

test("wrong password toasts the localized error", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Mật khẩu").fill("definitely wrong!!");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  // Next 16 also mounts its route announcer with role="alert", so pick the toast by its text.
  await expect(page.getByRole("alert").filter({ hasText: "Email hoặc mật khẩu không đúng." })).toBeVisible();
  await expect(page).toHaveURL(/\/login/);
});

test("empty fields are explained in place without a request", async ({ page }) => {
  await page.goto("/login");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText("Nhập email.")).toBeVisible();
  await expect(page.getByText("Nhập mật khẩu.")).toBeVisible();
});

test("theme and language are reachable from the sign-in page", async ({ page }) => {
  await page.goto("/login");
  await page.getByRole("button", { name: "Tuỳ chọn" }).click();
  await page.getByRole("menuitemradio", { name: "English" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Sign in" })).toBeVisible();
});
