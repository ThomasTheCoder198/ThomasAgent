import { expect, test, type Page } from "@playwright/test";

import { RUN_TIMEOUT, signIn } from "./helpers";

/** Review captures for the impeccable finish (excluded from the default run by `grepInvert`). */
const out = "../.impeccable/review";
const desktop = { width: 1440, height: 900 };
const mobile = { width: 390, height: 844 };
const wide = { width: 1920, height: 1080 };
const SETTLE_MS = 600;

// Stills cannot show motion; reduced motion makes every capture deterministic and exercises that path too.
test.use({ reducedMotion: "reduce" });

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

  await page.request.post(`${process.env.MOCK_CORE_URL ?? "http://localhost:8787"}/__mock/reset`);
  await page.goto("/agents");
  await shot(page, "agents-list-desktop-light");
  await page.goto("/agents/agent-phap-che?tab=overview");
  await shot(page, "agent-savebar-hidden-desktop-light");
  await page.getByRole("textbox", { name: "Tên" }).fill(" ");
  await shot(page, "agent-empty-name-desktop-light");
  await page.getByRole("button", { name: "Hoàn tác" }).click();
  await page.goto("/agents/agent-nhap-khau?tab=knowledge");
  await shot(page, "agent-unknown-kb-desktop-light");
  await page.goto("/agents/agent-phap-che?tab=tools");
  await shot(page, "agent-tools-desktop-light");
  await page
    .getByRole("group", { name: "Chính sách cho Huỷ hoá đơn" })
    .getByRole("radio", { name: "Hỏi trước" })
    .check({ force: true });
  await shot(page, "agent-unsaved-desktop-light");
  await page.getByRole("button", { name: "Hoàn tác" }).click();
  await theme(page, "dark");
  await page.goto("/agents/agent-phap-che?tab=models");
  await shot(page, "agent-models-desktop-dark");
  await theme(page, "light");
  await page.setViewportSize(mobile);
  await page.goto("/agents/agent-phap-che?tab=tools");
  await shot(page, "agent-tools-mobile-light");

  await page.setViewportSize(wide);
  await page.goto("/chat/c-hoan-tien-2026");
  await shot(page, "chat-done-wide-light");
  await page.getByRole("button", { name: /Thu gọn sidebar/ }).click();
  await shot(page, "chat-rail-wide-light");
  await page.getByRole("button", { name: /Mở rộng sidebar/ }).click();
});
