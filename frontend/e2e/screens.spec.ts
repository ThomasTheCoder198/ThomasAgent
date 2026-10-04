import { expect, test, type Page } from "@playwright/test";

import { RUN_TIMEOUT, signIn } from "./helpers";

/** Review captures for the impeccable finish (excluded from the default run by `grepInvert`). */
const out = "../.impeccable/review";
const desktop = { width: 1440, height: 900 };
const mobile = { width: 390, height: 844 };
const SETTLE_MS = 600;

async function theme(page: Page, value: "light" | "dark") {
  await page.evaluate((t) => {
    localStorage.setItem("theme", t);
    document.documentElement.setAttribute("data-theme", t);
  }, value);
}

async function shot(page: Page, name: string) {
  // The Next dev indicator is not part of the product UI.
  await page.addStyleTag({ content: "nextjs-portal{display:none!important}" });
  await page.waitForTimeout(SETTLE_MS);
  await page.screenshot({ path: `${out}/${name}.png` });
}

test("@screens capture", async ({ page }) => {
  test.setTimeout(180_000);
  await page.setViewportSize(desktop);
  await page.goto("/login");
  await shot(page, "login-desktop");
  await page.setViewportSize(mobile);
  await shot(page, "login-mobile");

  await page.setViewportSize(desktop);
  await signIn(page);
  await theme(page, "light");
  await shot(page, "chat-new-desktop-light");

  await page
    .getByLabel("Tin nhắn")
    .fill("Chính sách hoàn tiền 2026 thay đổi gì so với bản 2024? Sau đó tạo issue trên Linear.");
  await page.keyboard.press("Enter");
  await expect(page.getByText("Đang suy nghĩ…")).toBeVisible({ timeout: RUN_TIMEOUT });
  await page.screenshot({ path: `${out}/chat-thinking-desktop-light.png` });
  await expect(page.getByText("Tìm trong KB")).toBeVisible({ timeout: RUN_TIMEOUT });
  await page.screenshot({ path: `${out}/chat-streaming-desktop-light.png` });
  const approval = page.getByRole("region", { name: "Cần bạn duyệt" });
  await expect(approval).toBeVisible({ timeout: RUN_TIMEOUT });
  await shot(page, "chat-approval-desktop-light");
  await approval.getByRole("button", { name: "Cho phép", exact: true }).click();

  await expect(page.getByText("Đã tạo issue")).toBeVisible({ timeout: RUN_TIMEOUT });
  await page.getByRole("link", { name: "Chính sách hoàn tiền 2026", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Chính sách hoàn tiền 2026" })).toBeVisible();
  await page.getByRole("button", { name: "Nguồn 1" }).first().click();
  await shot(page, "chat-done-desktop-light");
  await page.getByRole("button", { name: /Đã suy nghĩ và dùng/ }).click();
  await shot(page, "chat-done-expanded-desktop-light");
  await page.getByRole("button", { name: /Đã suy nghĩ và dùng/ }).click();
  await theme(page, "dark");
  await shot(page, "chat-done-desktop-dark");

  await page.setViewportSize(mobile);
  await page.keyboard.press("Escape");
  await shot(page, "chat-done-mobile-dark");
  await page.getByRole("button", { name: "Nguồn 1" }).first().click();
  await shot(page, "evidence-sheet-mobile-dark");
  await page.keyboard.press("Escape");
  await theme(page, "light");
  await page.getByRole("button", { name: "Mở menu" }).click();
  await shot(page, "drawer-mobile-light");

  await page.setViewportSize(desktop);
  await page.goto("/knowledge-bases");
  await shot(page, "empty-kb-desktop-light");
});
