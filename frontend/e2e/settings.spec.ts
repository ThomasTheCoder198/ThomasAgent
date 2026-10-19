import { expect, test, type Page } from "@playwright/test";

import { signIn } from "./helpers";

// Desktop only: both projects edit the one fake catalog, and the phone layout is checked in the browser pass.
test.skip(({ isMobile }) => isMobile, "role edits are shared state; the phone layout is checked separately");
test.describe.configure({ mode: "serial" });

const MOCK_CORE_URL = process.env.MOCK_CORE_URL ?? "http://localhost:8787";

test.beforeEach(async ({ request }) => {
  await request.post(`${MOCK_CORE_URL}/__mock/reset?catalog=1`);
});

const row = (page: Page, title: string) =>
  page.locator("section", { has: page.getByRole("heading", { name: title, exact: true }) });

async function pickModel(page: Page, title: string, query: string, option: RegExp) {
  const scope = row(page, title);
  await scope.getByRole("combobox", { name: "Nhà cung cấp" }).click();
  await page.getByRole("option", { name: "OpenRouter" }).click();
  await scope.getByRole("combobox", { name: "Model" }).click();
  await page.getByPlaceholder("Tìm model…").fill(query);
  await page.getByRole("option", { name: option }).click();
}

const save = (page: Page) => page.getByRole("button", { name: /Lưu thay đổi/ });

test("the sidebar opens Settings with the four roles and the providers", async ({ page }) => {
  await signIn(page, "/chat");
  await page.getByRole("link", { name: "Cài đặt" }).click();
  await expect(page).toHaveURL(/\/settings$/);
  await expect(page.getByRole("heading", { level: 1, name: "Cài đặt model" })).toBeVisible();
  for (const title of ["Chat chính", "Chat nhanh", "Phân loại & kiểm tra trích dẫn", "Đọc ảnh"]) {
    await expect(row(page, title)).toBeVisible();
  }
  await expect(page.getByRole("heading", { name: "Nhà cung cấp đã kết nối" })).toBeVisible();
  await expect(page.getByText("Hội thoại tự xoá sau 30 ngày không hoạt động")).toBeVisible();
});

test("assigns a model to a role, tests it, and keeps it after reload", async ({ page }) => {
  await signIn(page, "/settings");
  await expect(row(page, "Đọc ảnh").getByText("Chưa gán — ảnh đính kèm sẽ bị từ chối")).toBeVisible();
  await pickModel(page, "Đọc ảnh", "claude", /Claude Sonnet 5\.5/);
  await row(page, "Đọc ảnh").getByRole("button", { name: "Thử" }).click();
  await expect(row(page, "Đọc ảnh").getByText(/Đã thử · 412 ms/)).toBeVisible();
  await save(page).click();
  await expect(page.getByText("Đã lưu vai trò model.")).toBeVisible();
  await page.reload();
  await expect(row(page, "Đọc ảnh").getByRole("combobox", { name: "Model" })).toContainText(
    "Claude Sonnet 5.5",
  );
});

test("a failed Test leaves Save reachable beside the persistent catalog error", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await signIn(page, "/settings");
  await page.route("**/api/v1/models/*/test", async (route) => {
    await route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "LLM_PROVIDER_REJECTED", message: "Rejected" } }),
    });
  });
  await pickModel(page, "Đọc ảnh", "claude", /Claude Sonnet 5\.5/);
  await row(page, "Đọc ảnh").getByRole("button", { name: "Thử" }).click();
  const toast = page
    .locator("[data-sonner-toast]")
    .filter({ hasText: "Provider từ chối key hoặc cấu hình." });
  await expect(toast).toBeVisible();
  const saveButton = save(page);
  await expect(saveButton).toBeVisible();
  const saveBox = await saveButton.boundingBox();
  expect(saveBox).not.toBeNull();
  await expect
    .poll(async () => {
      const toastBox = await toast.boundingBox();
      return toastBox ? toastBox.y + toastBox.height : Number.POSITIVE_INFINITY;
    })
    .toBeLessThanOrEqual(saveBox?.y ?? Number.NEGATIVE_INFINITY);
  await saveButton.click({ timeout: 3_000 });
  await expect(page.getByText("Đã lưu vai trò model.")).toBeVisible();
});

test("the model list offers fit models first and shows why the rest cannot be picked", async ({ page }) => {
  await signIn(page, "/settings");
  const scope = row(page, "Chat chính");
  await scope.getByRole("combobox", { name: "Model" }).click();
  await page.getByPlaceholder("Tìm model…").fill("llama");
  await expect(page.getByText("Không đủ năng lực cho Chat chính")).toBeVisible();
  await expect(page.getByRole("option", { name: /Llama 3\.3 70B/ })).toHaveAttribute("aria-disabled", "true");
});

test("an org overrides one role and inherits the rest", async ({ page }) => {
  await signIn(page, "/settings");
  await page.getByRole("tab", { name: /Công ty Demo/ }).click();
  await expect(row(page, "Chat nhanh").getByText(/Theo mặc định nền tảng: GPT-5 mini/)).toBeVisible();
  await row(page, "Chat nhanh").getByRole("button", { name: "Ghi đè" }).click();
  await row(page, "Chat nhanh").getByRole("combobox", { name: "Model" }).click();
  await page.getByRole("option", { name: /Claude Sonnet 5\.5/ }).click();
  await save(page).click();
  await expect(page.getByText("Đã lưu vai trò model.")).toBeVisible();
  await expect(page.getByRole("tab", { name: /1 ghi đè/ })).toBeVisible();
  await row(page, "Chat nhanh").getByRole("button", { name: "Bỏ ghi đè" }).click();
  await save(page).click();
  await expect(page.getByRole("tab", { name: /ghi đè/ })).toHaveCount(0);
  await expect(row(page, "Chat nhanh").getByText(/Theo mặc định nền tảng: GPT-5 mini/)).toBeVisible();
});

test("leaving a tab with unsaved edits asks first", async ({ page }) => {
  await signIn(page, "/settings");
  await row(page, "Chat nhanh").getByRole("combobox", { name: "Model" }).click();
  await page.getByRole("option", { name: /Claude Sonnet 5\.5/ }).click();
  await page.getByRole("tab", { name: /Công ty Demo/ }).click();
  await expect(page.getByRole("alertdialog", { name: "Bỏ thay đổi chưa lưu?" })).toBeVisible();
  await page.getByRole("button", { name: "Tiếp tục sửa" }).click();
  await expect(page.getByRole("tab", { name: /Mặc định nền tảng/ })).toHaveAttribute("aria-selected", "true");
});

test("adds a provider, asking for the address only where it is needed", async ({ page }) => {
  await signIn(page, "/settings");
  await page.getByRole("button", { name: "Thêm nhà cung cấp" }).click();
  await expect(page.getByRole("textbox", { name: "Base URL" })).toHaveCount(0);
  await page.getByRole("combobox", { name: "Loại" }).click();
  await page.getByRole("option", { name: "OpenAI-compatible" }).click();
  await expect(page.getByRole("textbox", { name: "Base URL" })).toBeVisible();
  await page.getByRole("textbox", { name: "Tên hiển thị" }).fill("Máy chủ thử");
  await page.getByRole("textbox", { name: "Base URL" }).fill("http://localhost:9000/v1");
  await page.getByRole("button", { name: "Lưu & đồng bộ" }).click();
  await expect(page.getByText("Đã lưu nhà cung cấp Máy chủ thử.")).toBeVisible();
  await expect(page.getByRole("list", { name: "Nhà cung cấp đã kết nối" })).toContainText("Máy chủ thử");
});

test("refuses to delete a provider a role still uses, and the dialog stays", async ({ page }) => {
  await signIn(page, "/settings");
  await page.getByRole("button", { name: "Thao tác với OpenRouter" }).click();
  await page.getByRole("menuitem", { name: "Xoá nhà cung cấp" }).click();
  await page.getByRole("button", { name: "Xoá", exact: true }).click();
  await expect(page.getByText(/đang được gán/)).toBeVisible();
  await expect(page.getByRole("alertdialog")).toBeVisible();
});
