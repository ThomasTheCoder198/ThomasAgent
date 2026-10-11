import { expect, test, type Page, type TestInfo } from "@playwright/test";

import { signIn } from "./helpers";

/**
 * The fake core is shared by the desktop and mobile workers and by other specs, which may run at the same time,
 * so nothing here resets it: every name and every file's bytes are unique per project and run.
 */
const READY_TIMEOUT_MS = 45_000;
const NO_REQUEST_WINDOW_MS = 1000;
const S3_UPLOAD = /__mock\/s3\/kb-raw$/;

const unique = (testInfo: TestInfo, label: string) => `${label}-${testInfo.project.name}-${Date.now()}`;

/** On phones the sidebar is a drawer: open it. Decided by project, because the button renders after hydration. */
async function openNav(page: Page, testInfo: TestInfo) {
  if (testInfo.project.name === "mobile") await page.getByRole("button", { name: "Mở menu" }).click();
}

async function openDemoDefaultKb(page: Page, testInfo: TestInfo) {
  await signIn(page);
  await openNav(page, testInfo);
  await page
    .getByRole("navigation", { name: "Điều hướng chính" })
    .getByRole("link", { name: /Knowledge Base/ })
    .click();
  await expect(page).toHaveURL(/\/orgs\/[^/]+\/knowledge-bases$/);
  await page.getByRole("link", { name: /Mặc định/ }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Mặc định" })).toBeVisible();
}

test("the sidebar opens the org's Knowledge Base list with the Default plate", async ({ page }, testInfo) => {
  await signIn(page);
  await openNav(page, testInfo);
  await page
    .getByRole("navigation", { name: "Điều hướng chính" })
    .getByRole("link", { name: /Knowledge Base/ })
    .click();
  await expect(page.getByRole("heading", { level: 1, name: "Knowledge Base" })).toBeVisible();
  const row = page.getByRole("link", { name: /Mặc định/ }).first();
  await expect(row).toBeVisible();
  await expect(row.getByText("Mặc định")).toHaveCount(2);
});

test("uploads a file: presign, bytes straight to storage, confirm, then it reaches Sẵn sàng", async ({
  page,
}, testInfo) => {
  const name = `${unique(testInfo, "upload")}.md`;
  const marker = `body-${unique(testInfo, "marker")}`;
  const apiBodies: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v1") && (request.postData() ?? "").includes(marker))
      apiBodies.push(request.url());
  });
  await openDemoDefaultKb(page, testInfo);
  const toStorage = page.waitForRequest(S3_UPLOAD);
  await page
    .locator('input[type="file"]')
    .setInputFiles({ name, mimeType: "text/markdown", buffer: Buffer.from(`# ${marker}\n`) });
  const storageRequest = await toStorage;
  expect(storageRequest.method()).toBe("POST");
  expect(await storageRequest.headerValue("cookie")).toBeNull();
  await expect(page.getByRole("listitem").filter({ hasText: name }).getByText("Sẵn sàng")).toBeVisible({
    timeout: READY_TIMEOUT_MS,
  });
  expect(apiBodies).toEqual([]);
});

test("rejects an unsupported type locally, without any request", async ({ page }, testInfo) => {
  const uploads: string[] = [];
  page.on("request", (request) => {
    if (request.url().endsWith("/uploads")) uploads.push(request.url());
  });
  await openDemoDefaultKb(page, testInfo);
  await page
    .locator('input[type="file"]')
    .setInputFiles({ name: "virus.exe", mimeType: "application/octet-stream", buffer: Buffer.from("x") });
  await expect(page.getByText("Không hỗ trợ loại tệp này.")).toBeVisible();
  await page.waitForTimeout(NO_REQUEST_WINDOW_MS);
  expect(uploads).toEqual([]);
});

test("the default Knowledge Base cannot be deleted", async ({ page }, testInfo) => {
  await openDemoDefaultKb(page, testInfo);
  // File rows have their own Delete buttons; the sign band is the <header>.
  const sign = page.locator("header");
  await expect(sign.getByRole("button", { name: "Đổi tên" })).toBeVisible();
  await expect(sign.getByRole("button", { name: "Xoá" })).toHaveCount(0);
});

test("creates a Knowledge Base and lands on its page", async ({ page }, testInfo) => {
  const name = unique(testInfo, "E2E KB");
  await signIn(page);
  await openNav(page, testInfo);
  await page
    .getByRole("navigation", { name: "Điều hướng chính" })
    .getByRole("link", { name: /Knowledge Base/ })
    .click();
  await page.getByRole("button", { name: "Knowledge Base mới" }).click();
  await page.getByLabel("Tên").fill(name);
  await page.getByRole("button", { name: "Tạo", exact: true }).click();
  await expect(page).toHaveURL(/\/orgs\/[^/]+\/knowledge-bases\/[^/]+$/);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
});

test("an org ragx could not prepare shows the red station, and Retry opens its Knowledge Bases", async ({
  page,
}, testInfo) => {
  const name = `${unique(testInfo, "Beta")} !role`;
  await signIn(page);
  await openNav(page, testInfo);
  await page.getByRole("button", { name: /Đổi tổ chức/ }).click();
  await page.getByRole("menuitem", { name: "Tổ chức mới" }).click();
  await page.getByLabel("Tên").fill(name);
  await page.getByRole("button", { name: "Tạo tổ chức" }).click();
  await expect(page.getByText("LLM_ROLE_NOT_ASSIGNED")).toBeVisible();
  await page.getByRole("button", { name: "Thử lại" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Knowledge Base" })).toBeVisible();
  await expect(page.getByRole("link", { name: /Mặc định/ })).toBeVisible();
});

test("downloads a file with its own name", async ({ page }, testInfo) => {
  // Uploads from other tests push the seeded files off the first page, so this test brings its own file.
  const name = `${unique(testInfo, "download")}.md`;
  await openDemoDefaultKb(page, testInfo);
  await page.locator('input[type="file"]').setInputFiles({
    name,
    mimeType: "text/markdown",
    buffer: Buffer.from(`# ${name}
`),
  });
  const row = page
    .getByRole("listitem")
    .filter({ hasText: name })
    .filter({ has: page.getByRole("button", { name: "Tải xuống" }) });
  const download = page.waitForEvent("download");
  await row.getByRole("button", { name: "Tải xuống" }).click();
  expect((await download).suggestedFilename()).toBe(name);
});

test("a file storage refuses shows the S3 code and no request id", async ({ page }, testInfo) => {
  const name = `${unique(testInfo, "toolarge")}.md`;
  // The presign asks for 1 byte, so the policy's exact size range refuses the real file at storage.
  await page.route(/\/uploads$/, (route) =>
    route.continue({ postData: JSON.stringify({ name, sizeBytes: 1 }) }),
  );
  await openDemoDefaultKb(page, testInfo);
  await page
    .locator('input[type="file"]')
    .setInputFiles({ name, mimeType: "text/markdown", buffer: Buffer.from("more than one byte") });
  const alert = page.getByRole("alert").filter({ hasText: "EntityTooLarge" });
  await expect(alert).toContainText("Kho lưu trữ từ chối tệp (EntityTooLarge)");
  await expect(alert).not.toContainText("Mã yêu cầu");
  await page.screenshot({ path: "../docs/superpowers/reports/m1-2-ui-kb-storage-refused-vi.png" });
});

test("ragx down shows the retryable service error, decided by status", async ({ page }, testInfo) => {
  const name = `${unique(testInfo, "Down")} !down`;
  await signIn(page);
  await openNav(page, testInfo);
  await page.getByRole("button", { name: /Đổi tổ chức/ }).click();
  await page.getByRole("menuitem", { name: "Tổ chức mới" }).click();
  await page.getByLabel("Tên").fill(name);
  await page.getByRole("button", { name: "Tạo tổ chức" }).click();
  await expect(page.getByRole("heading", { name: "Không tải được trang" })).toBeVisible();
  await expect(page.getByText(/Mã yêu cầu/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Thử lại" })).toBeVisible();
});
