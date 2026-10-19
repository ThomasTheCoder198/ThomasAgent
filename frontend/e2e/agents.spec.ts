import { expect, test, type Page, type TestInfo } from "@playwright/test";

import { signIn } from "./helpers";

// The tests of one project share one Agent and reset it before each test, so they run in order.
test.describe.configure({ mode: "serial" });

/** The fake core is shared by the desktop and mobile workers, so each project edits its own seeded Agent. */
const AGENT_BY_PROJECT: Record<string, string> = { desktop: "Trợ lý pháp chế", mobile: "CSKH đơn hàng" };
const MOCK_CORE_URL = process.env.MOCK_CORE_URL ?? "http://localhost:8787";
const UNKNOWN_KB_ID = "00000000-0000-4000-8000-000000000001";

function agentFor(testInfo: TestInfo) {
  return AGENT_BY_PROJECT[testInfo.project.name] ?? "Trợ lý pháp chế";
}

/** Seeded Agent ids are not fixed, so the tests open an Agent the way the owner does: from the list. */
async function openAgent(page: Page, name: string, tab: string) {
  await signIn(page, "/agents");
  await page.getByRole("link", { name: new RegExp(name) }).click();
  await expect(page).toHaveURL(/\/agents\/[^/?]+$/);
  await page.goto(`${new URL(page.url()).pathname}?tab=${tab}`);
}

function saveButton(page: Page) {
  return page.getByRole("button", { name: /Lưu thay đổi|Save changes/ });
}

// Scoped to this project's Agent: the other project and the other specs use the same fake in parallel.
test.beforeEach(async ({ request }, testInfo) => {
  await request.post(`${MOCK_CORE_URL}/__mock/reset?agent=${encodeURIComponent(agentFor(testInfo))}`);
});

test("opens an Agent from its row in the list", async ({ page }, testInfo) => {
  await signIn(page, "/agents");
  await page.getByRole("link", { name: new RegExp(agentFor(testInfo)) }).click();
  await expect(page).toHaveURL(/\/agents\/[^/?]+$/);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Tổng quan" })).toHaveAttribute("aria-selected", "true");
});

test("lists the read-only KB tools as automatic, with nothing to edit", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "tools");
  const tools = page.getByRole("list").filter({ hasText: "kb_search" });
  await expect(tools.getByRole("listitem")).toHaveCount(4);
  await expect(tools.getByText("Tự động")).toHaveCount(4);
  await expect(page.getByRole("radiogroup")).toHaveCount(0);
});

test("changes the Knowledge Base binding and keeps it", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "knowledge");
  const picker = page.getByRole("combobox", { name: "Knowledge" });
  // Seeded Agents are bound to different KBs; pick the one this Agent does not have (once the picker shows its KB).
  await expect(picker).toContainText(/Mặc định|Chính sách/);
  const other = (await picker.textContent())?.includes("Chính sách") ? /Mặc định/ : /Chính sách/;
  await picker.click();
  await page.getByRole("option", { name: other }).click();
  await saveButton(page).click();
  await expect(page.getByText("Đã lưu")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("combobox", { name: "Knowledge" })).toHaveText(other);
});

test("overrides the chat model and keeps it after reload", async ({ page }, testInfo) => {
  await openAgent(page, agentFor(testInfo), "models");
  const chat = page.getByRole("combobox", { name: "Chat", exact: true });
  await chat.click();
  await page.getByRole("option", { name: "Claude Sonnet 5.5 · anthropic/claude-sonnet-5.5" }).click();
  await saveButton(page).click();
  await expect(page.getByText("Đã lưu")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("combobox", { name: "Chat", exact: true })).toHaveText(/Claude Sonnet 5\.5 ·/);
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
  await page.route("**/api/v1/orgs/*/agents/*", async (route) => {
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

test("core refuses to bind an Agent to another org's real Knowledge Base", async ({ page }, testInfo) => {
  await signIn(page, "/agents");
  const orgs = await (await page.request.get("/api/v1/orgs")).json();
  const orgId = orgs.data[0].id as string;
  const agents = await (await page.request.get(`/api/v1/orgs/${orgId}/agents`)).json();
  const agent = (agents.data as { id: string; name: string }[]).find((a) => a.name === agentFor(testInfo));
  const csrf = (await page.context().cookies()).find((c) => c.name === "thomas_csrf")?.value ?? "";
  const headers = { "X-CSRF-Token": csrf, "Content-Type": "application/json" };
  const suffix = `${testInfo.project.name}-${Date.now()}`;
  const created = await page.request.post("/api/v1/orgs", {
    headers,
    data: { name: `Foreign KB ${suffix}`, externalParsing: false },
  });
  expect(created.status()).toBe(201);
  const foreignOrgId = (await created.json()).data.id as string;
  const kbCreated = await page.request.post(`/api/v1/orgs/${foreignOrgId}/apps/ragx/knowledge-bases`, {
    headers,
    data: { name: `KB ${suffix}` },
  });
  expect(kbCreated.status()).toBe(201);
  const foreignKbId = (await kbCreated.json()).data.id as string;
  const res = await page.request.patch(`/api/v1/orgs/${orgId}/agents/${agent?.id}`, {
    headers,
    data: { kbId: foreignKbId },
  });
  expect(res.status()).toBe(422);
  expect((await res.json()).error.code).toBe("AGENT_KB_INVALID");
});

test("core refuses to bind an Agent to an unknown Knowledge Base", async ({ page }, testInfo) => {
  await signIn(page, "/agents");
  const orgs = await (await page.request.get("/api/v1/orgs")).json();
  const orgId = orgs.data[0].id as string;
  const agents = await (await page.request.get(`/api/v1/orgs/${orgId}/agents`)).json();
  const agent = (agents.data as { id: string; name: string }[]).find((a) => a.name === agentFor(testInfo));
  const csrf = (await page.context().cookies()).find((c) => c.name === "thomas_csrf")?.value ?? "";
  const res = await page.request.patch(`/api/v1/orgs/${orgId}/agents/${agent?.id}`, {
    headers: { "X-CSRF-Token": csrf, "Content-Type": "application/json" },
    data: { kbId: UNKNOWN_KB_ID },
  });
  expect(res.status()).toBe(422);
  expect((await res.json()).error.code).toBe("AGENT_KB_INVALID");
});

test("marks a binding to a deleted Knowledge Base as unknown", async ({ page }, testInfo) => {
  await signIn(page, "/agents");
  // A private org keeps the shared fake's other orgs and KBs out of this test.
  const suffix = `${testInfo.project.name}-${Date.now()}`;
  const csrf = (await page.context().cookies()).find((c) => c.name === "thomas_csrf")?.value ?? "";
  const headers = { "X-CSRF-Token": csrf, "Content-Type": "application/json" };
  const created = await page.request.post("/api/v1/orgs", {
    headers,
    data: { name: `KB gone ${suffix}`, externalParsing: false },
  });
  const orgId = (await created.json()).data.id as string;
  const kbs = `/api/v1/orgs/${orgId}/apps/ragx/knowledge-bases`;
  const kb = (await (await page.request.post(kbs, { headers, data: { name: "Tạm" } })).json()).data;
  const made = await page.request.post(`/api/v1/orgs/${orgId}/agents`, {
    headers,
    data: { name: `Agent ${suffix}`, kbId: kb.id },
  });
  const agentId = (await made.json()).data.id as string;
  expect((await page.request.delete(`${kbs}/${kb.id}`, { headers })).status()).toBe(204);

  // Opening the org's page makes it the current org for /agents.
  await page.goto(`/orgs/${orgId}/knowledge-bases`);
  await page.goto(`/agents/${agentId}?tab=knowledge`);
  await expect(page.getByRole("combobox", { name: "Knowledge" })).toHaveText(/Knowledge Base không xác định/);
  await expect(page.getByRole("alert").filter({ hasText: "đã bị xoá" })).toBeVisible();
});

test("creates an agent on a KB and deletes it again", async ({ page }, testInfo) => {
  // One name per project: the desktop and mobile workers share the fake core's org.
  const name = `Agent e2e ${testInfo.project.name}`;
  await signIn(page, "/agents");
  await page.getByRole("button", { name: "Tạo agent" }).click();
  const dialog = page.getByRole("dialog", { name: "Tạo agent mới" });
  await dialog.getByRole("textbox", { name: "Tên" }).fill(name);
  await dialog.getByRole("button", { name: "Tạo", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();

  await page.getByRole("button", { name: "Xoá agent" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Xoá", exact: true }).click();
  await expect(page).toHaveURL(/\/agents$/);
  await expect(page.getByRole("link", { name: new RegExp(name) })).toHaveCount(0);
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
