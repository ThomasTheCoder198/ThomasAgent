import { expect, test } from "@playwright/test";

import { RUN_TIMEOUT, signIn } from "./helpers";

/**
 * Drives the whole agent loop over the real UI Message Stream: stations stream in, the risky tool waits
 * for approval, the run folds into its summary, and a citation opens its source block.
 * Requires a core (or the fake core in `mock-core/`) that serves the chat stream.
 */
test("streams a run, waits for approval, then cites its evidence", async ({ page }, testInfo) => {
  await signIn(page);
  await expect(page.getByRole("heading", { level: 2, name: "Hôm nay bạn cần tra gì?" })).toBeVisible();
  await expect(page.getByRole("list", { name: "Nguồn công cụ" })).toBeVisible();
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
  // From xl the evidence is a docked column (complementary); below it a modal sheet.
  const evidence = page.getByRole(testInfo.project.name === "mobile" ? "dialog" : "complementary", {
    name: "Bằng chứng",
  });
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

test("Stop rejects another org's run and an unknown run", async ({ page }, testInfo) => {
  await signIn(page);
  const csrf = (await page.context().cookies()).find((cookie) => cookie.name === "thomas_csrf")?.value ?? "";
  const headers = { "X-CSRF-Token": csrf, "Content-Type": "application/json" };
  const orgId = ((await (await page.request.get("/api/v1/orgs")).json()).data as { id: string }[])[0]?.id;
  expect(orgId).toBeTruthy();
  const agentId = (
    (await (await page.request.get(`/api/v1/orgs/${orgId}/agents`)).json()).data as { id: string }[]
  )[0]?.id;
  expect(agentId).toBeTruthy();
  const runId = `R-stop-scope-${testInfo.project.name}-${Date.now()}`;
  const chat = await page.request.post(`/api/v1/orgs/${orgId}/chat`, {
    headers,
    data: {
      id: `chat-stop-scope-${testInfo.project.name}-${Date.now()}`,
      agentId,
      trigger: "submit-message",
      message: {
        id: `msg-${Date.now()}`,
        role: "user",
        parts: [{ type: "text", text: "Test Stop" }],
        metadata: { runId },
      },
    },
  });
  expect(chat.status()).toBe(200);
  const created = await page.request.post("/api/v1/orgs", {
    headers,
    data: { name: `Stop scope ${testInfo.project.name}-${Date.now()}`, externalParsing: false },
  });
  expect(created.status()).toBe(201);
  const foreignOrgId = (await created.json()).data.id as string;

  const foreign = await page.request.post(`/api/v1/orgs/${foreignOrgId}/runs/${runId}/stop`, { headers });
  expect(foreign.status()).toBe(409);
  expect((await foreign.json()).error.code).toBe("RUN_NOT_ACTIVE");
  const unknown = await page.request.post(`/api/v1/orgs/${orgId}/runs/R-never-started/stop`, { headers });
  expect(unknown.status()).toBe(409);
  expect((await unknown.json()).error.code).toBe("RUN_NOT_ACTIVE");
  const own = await page.request.post(`/api/v1/orgs/${orgId}/runs/${runId}/stop`, { headers });
  expect(own.status()).toBe(202);
  expect((await own.json()).data.status).toBe("completed");
});
