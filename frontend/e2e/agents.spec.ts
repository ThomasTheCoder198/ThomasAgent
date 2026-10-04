import { expect, test, type Page, type TestInfo } from "@playwright/test";

import { signIn } from "./helpers";

/** The fake core is shared by the desktop and mobile workers, so each project edits its own Agent. */
const AGENT_BY_PROJECT: Record<string, string> = { desktop: "agent-phap-che", mobile: "agent-cskh" };
const MOCK_CORE_URL = process.env.MOCK_CORE_URL ?? "http://localhost:8787";
const UNKNOWN_KB_AGENT = "agent-nhap-khau";

function agentFor(testInfo: TestInfo) {
  return AGENT_BY_PROJECT[testInfo.project.name] ?? "agent-phap-che";
}

async function openAgent(page: Page, agentId: string, tab: string) {
  await signIn(page, "/agents");
  await page.goto(`/agents/${agentId}?tab=${tab}`);
}

function saveButton(page: Page) {
  return page.getByRole("button", { name: /Lưu thay đổi|Save changes/ });
}

test.beforeEach(async ({ request }, testInfo) => {
  await request.post(`${MOCK_CORE_URL}/__mock/reset?agent=${agentFor(testInfo)}`);
});

test("edits a tool policy, saves it, and the change survives a reload", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "tools");
  const policy = page.getByRole("group", { name: /Gửi email/ });
  await policy.getByRole("radio", { name: "Tự động" }).check({ force: true });
  await saveButton(page).click();
  await expect(page.getByText("Đã lưu")).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("group", { name: /Gửi email/ }).getByRole("radio", { name: "Tự động" }),
  ).toBeChecked();
});

test("changes the Knowledge Base binding and keeps it", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "knowledge");
  const target = page.getByRole("radio", { name: /Hợp đồng nhà cung cấp/ });
  await target.check({ force: true });
  await saveButton(page).click();
  await expect(page.getByText("Đã lưu")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("radio", { name: /Hợp đồng nhà cung cấp/ })).toBeChecked();
});

test("overrides the chat model and keeps it after reload", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "models");
  const chat = page.getByRole("combobox", { name: "Chat", exact: true });
  await chat.selectOption({ label: "Claude Sonnet 5.5 · anthropic/claude-sonnet-5.5" });
  await saveButton(page).click();
  await expect(page.getByText("Đã lưu")).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("combobox", { name: "Chat", exact: true }).locator("option:checked"),
  ).toHaveText(/Claude Sonnet 5\.5 ·/);
});

test("blocks saving an empty name and says why", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "overview");
  await page.getByRole("textbox", { name: "Tên" }).fill("   ");
  await expect(page.getByRole("textbox", { name: "Tên" })).toHaveAttribute("aria-invalid", "true");
  await expect(saveButton(page)).toBeDisabled();
});

test("discarding restores the stored instructions", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "instructions");
  const box = page.getByRole("textbox", { name: "Instructions" });
  const original = await box.inputValue();
  await box.fill("Tạm thời");
  await page.getByRole("button", { name: /Hoàn tác|Discard/ }).click();
  await expect(box).toHaveValue(original);
});

test("shows the server's refusal when core rejects the save", async ({ page }, testInfo) => {
  await page.route("**/api/v1/agents/*", async (route) => {
    if (route.request().method() !== "PATCH") return route.fallback();
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "FORBIDDEN", message: "Bạn không có quyền thực hiện thao tác này." },
      }),
    });
  });
  await openAgent(page, agentFor(testInfo), "instructions");
  await page.getByRole("textbox", { name: "Instructions" }).fill("Đổi để thử lỗi");
  await saveButton(page).click();
  await expect(page.getByText("Bạn không có quyền thực hiện thao tác này.")).toBeVisible();
});

test("core refuses to bind an Agent to another org's Knowledge Base", async ({ page }, testInfo) => {
  await signIn(page, "/agents");
  const csrf = (await page.context().cookies()).find((c) => c.name === "thomas_csrf")?.value ?? "";
  const res = await page.request.patch(`/api/v1/agents/${agentFor(testInfo)}`, {
    headers: { "X-CSRF-Token": csrf, "Content-Type": "application/json" },
    data: { kbId: "kb-org-khac" },
  });
  expect(res.status()).toBe(403);
  expect((await res.json()).error.code).toBe("FORBIDDEN");
});

test("marks a binding to a Knowledge Base outside this org as unknown", async ({ page }) => {
  await openAgent(page, UNKNOWN_KB_AGENT, "knowledge");
  await expect(page.getByRole("radio", { name: /Knowledge Base không xác định/ })).toBeChecked();
});

test("collapses the desktop sidebar to an icon rail and keeps it", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === "mobile", "the rail is desktop-only; phones use the drawer");
  await signIn(page);
  await page.getByRole("button", { name: /Thu gọn sidebar/ }).click();
  await expect(page.getByRole("button", { name: /Mở rộng sidebar/ })).toBeVisible();
  await expect(page.getByRole("link", { name: "Agents" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("button", { name: /Mở rộng sidebar/ })).toBeVisible();
  await page.getByRole("button", { name: /Mở rộng sidebar/ }).click();
  await expect(page.getByRole("button", { name: /Thu gọn sidebar/ })).toBeVisible();
});

test("Ctrl+B typed inside a field does not collapse the sidebar", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === "mobile", "the rail is desktop-only");
  await openAgent(page, agentFor(testInfo), "instructions");
  await page.getByRole("textbox", { name: "Instructions" }).press("Control+b");
  await expect(page.getByRole("button", { name: /Thu gọn sidebar/ })).toBeVisible();
});
