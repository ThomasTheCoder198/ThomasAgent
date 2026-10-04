import assert from "node:assert/strict";
import { test } from "node:test";

import { buildRunOptions, runCollection } from "./run.mjs";

const fixtureEnvironment = {
  CORE_API_TEST_BASE_URL: "http://127.0.0.1:19080",
  CORE_AUTH_OWNER_EMAIL: "owner@example.com",
  CORE_AUTH_OWNER_PASSWORD: "fixture password for API tests",
};

test("runner passes environment credentials to Newman without command-line arguments", () => {
  const options = buildRunOptions(fixtureEnvironment);
  assert.equal(
    options.envVar.find((variable) => variable.key === "baseUrl").value,
    fixtureEnvironment.CORE_API_TEST_BASE_URL,
  );
  assert.equal(
    options.envVar.find((variable) => variable.key === "ownerPassword").value,
    fixtureEnvironment.CORE_AUTH_OWNER_PASSWORD,
  );
  assert.deepEqual(options.reporters, []);
  assert.ok(!options.folder.includes("Internal"));
  assert.deepEqual(options.folder.slice(-2), ["Lifecycle", "Logout"]);
});

test("runner includes internal resolution when a service token is supplied", () => {
  const options = buildRunOptions({
    ...fixtureEnvironment,
    CORE_SERVICE_TOKEN: "fixture service token",
  });
  assert.ok(options.folder.includes("Internal"));
  assert.equal(
    options.envVar.find((variable) => variable.key === "serviceToken").value,
    "fixture service token",
  );
});

test("runner rejects missing owner credentials before executing requests", () => {
  assert.throws(() => buildRunOptions({}), /CORE_AUTH_OWNER_EMAIL/);
  assert.throws(
    () => buildRunOptions({ CORE_AUTH_OWNER_EMAIL: "owner@example.com" }),
    /CORE_AUTH_OWNER_PASSWORD/,
  );
});

test("runner propagates failures and returns success only when all Newman checks pass", async () => {
  const summary = { run: { failures: [], stats: {} } };
  assert.equal(
    await runCollection(fixtureEnvironment, (_options, callback) =>
      callback(null, summary),
    ),
    summary,
  );
  await assert.rejects(
    runCollection(fixtureEnvironment, (_options, callback) =>
      callback(new Error("transport failed")),
    ),
    /transport failed/,
  );
  const failing = {
    run: { failures: [{ error: { test: "expected status" } }], stats: {} },
  };
  assert.equal(
    await runCollection(fixtureEnvironment, (_options, callback) =>
      callback(null, failing),
    ),
    failing,
  );
});
