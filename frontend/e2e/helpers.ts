import { expect, type Page } from "@playwright/test";

export const owner = {
  email: process.env.CORE_AUTH_OWNER_EMAIL ?? "",
  password: process.env.CORE_AUTH_OWNER_PASSWORD ?? "",
};

export async function signIn(page: Page, next = "/chat") {
  await page.goto(`/login?next=${encodeURIComponent(next)}`);
  await page.getByLabel("Email").fill(owner.email);
  await page.getByLabel("Mật khẩu").fill(owner.password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page).toHaveURL(new RegExp(`${next}$`));
}

/** Scripted runs take several seconds at normal speed. */
export const RUN_TIMEOUT = 30_000;
