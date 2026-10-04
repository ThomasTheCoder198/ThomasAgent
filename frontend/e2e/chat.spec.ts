import { expect, test } from "@playwright/test";

import { RUN_TIMEOUT, signIn } from "./helpers";

/**
 * Drives the whole agent loop over the real UI Message Stream: stations stream in, the risky tool waits
 * for approval, the run folds into its summary, and a citation opens its source block.
 * Requires a core (or the fake core in `mock-core/`) that serves the chat stream.
 */
test("streams a run, waits for approval, then cites its evidence", async ({ page }) => {
  await signIn(page);
  await page.getByLabel("Tin nhắn").fill("Chính sách hoàn tiền 2026 thay đổi gì?");
  await page.keyboard.press("Enter");

  await expect(page).toHaveURL(/\/chat\/[\w-]+$/);
  const approval = page.getByRole("region", { name: "Cần bạn duyệt" });
  await expect(approval).toBeVisible({ timeout: RUN_TIMEOUT });
  await approval.getByRole("button", { name: "Cho phép", exact: true }).click();

  await expect(page.getByRole("button", { name: /Đã suy nghĩ và dùng 3 công cụ/ })).toBeVisible({
    timeout: RUN_TIMEOUT,
  });
  await page.getByRole("button", { name: "Nguồn 1" }).first().click();
  const evidence = page.getByRole("complementary", { name: "Bằng chứng" });
  await expect(evidence.getByRole("heading", { name: "chinh-sach-hoan-tien-2026.pdf" })).toBeVisible();
  await expect(evidence.getByText(/14 \(mười bốn\) ngày/)).toBeVisible();
});

test("opens a stored conversation from recents", async ({ page }, testInfo) => {
  await signIn(page);
  if (testInfo.project.name === "mobile") await page.getByRole("button", { name: "Mở menu" }).click();
  await page.getByRole("link", { name: "Chính sách hoàn tiền 2026", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Chính sách hoàn tiền 2026" })).toBeVisible();
  await expect(page.getByRole("table")).toContainText("14 ngày");
});
