import assert from "node:assert/strict";
import { test } from "node:test";

import { postprocess } from "./postprocess.mjs";

const raw = {
  info: { name: "ThomasAgent Core API" },
  item: [
    {
      name: "Providers",
      item: [
        {
          name: "create Provider",
          request: {
            method: "POST",
            url: { host: ["{{baseUrl}}"], path: ["api", "v1", "providers"] },
          },
        },
      ],
    },
    {
      name: "Auth",
      item: [
        {
          name: "login",
          request: {
            method: "POST",
            url: {
              host: ["{{baseUrl}}"],
              path: ["api", "v1", "auth", "login"],
            },
          },
        },
      ],
    },
    {
      name: "Health",
      item: [
        {
          name: "healthz",
          request: {
            method: "GET",
            url: { host: ["{{baseUrl}}"], path: ["healthz"] },
          },
        },
      ],
    },
  ],
  variable: [],
};

test("orders folders Health, Auth first", () => {
  const out = postprocess(structuredClone(raw));
  assert.deepEqual(out.item.map((f) => f.name).slice(0, 2), ["Health", "Auth"]);
});

test("adds CSRF pre-request and response-format tests at collection level", () => {
  const out = postprocess(structuredClone(raw));
  const pre = out.event
    .find((e) => e.listen === "prerequest")
    .script.exec.join("\n");
  const tests = out.event
    .find((e) => e.listen === "test")
    .script.exec.join("\n");
  assert.match(pre, /X-CSRF-Token/);
  assert.match(tests, /response format/);
});

test("login stores csrfToken and create requests store ids", () => {
  const out = postprocess(structuredClone(raw));
  const login = out.item.find((f) => f.name === "Auth").item[0];
  assert.match(login.event[0].script.exec.join("\n"), /csrfToken/);
  const create = out.item.find((f) => f.name === "Providers").item[0];
  assert.match(create.event[0].script.exec.join("\n"), /providerId/);
});

test("is deterministic: random generator ids are removed", () => {
  const withIds = structuredClone(raw);
  withIds.info._postman_id = "random-1";
  withIds.item[0].id = "random-2";
  const a = JSON.stringify(postprocess(structuredClone(withIds)));
  withIds.info._postman_id = "random-3";
  withIds.item[0].id = "random-4";
  const b = JSON.stringify(postprocess(structuredClone(withIds)));
  assert.equal(a, b);
  assert.doesNotMatch(a, /random-/);
});

test("declares collection variables", () => {
  const out = postprocess(structuredClone(raw));
  const names = out.variable.map((v) => v.key);
  for (const key of ["baseUrl", "csrfToken", "providerId", "modelId"])
    assert.ok(names.includes(key), key);
});

test("keeps required query examples while disabling optional generated filters", () => {
  const input = structuredClone(raw);
  input.item.push({
    name: "Internal",
    item: [
      {
        name: "resolve",
        request: {
          method: "GET",
          url: {
            path: ["internal", "models", "resolve"],
            query: [
              {
                key: "role",
                value: "chat.fast",
                description: { content: "(Required)" },
              },
              {
                key: "capability",
                value: "random-choice",
                description: { content: "" },
              },
            ],
          },
        },
      },
    ],
  });
  const output = postprocess(input);
  const query = output.item.find((folder) => folder.name === "Internal").item[0]
    .request.url.query;
  assert.equal(query[0].value, "chat.fast");
  assert.equal(query[0].disabled, false);
  assert.equal(query[1].value, "");
  assert.equal(query[1].disabled, true);
});

test("runs deletion checks and logout after model role assignment", () => {
  const input = structuredClone(raw);
  input.item
    .find((folder) => folder.name === "Auth")
    .item.push({
      name: "logout",
      request: {
        method: "POST",
        url: { path: ["api", "v1", "auth", "logout"] },
      },
    });
  input.item
    .find((folder) => folder.name === "Providers")
    .item.push({
      name: "delete provider",
      request: {
        method: "DELETE",
        url: { path: ["api", "v1", "providers", ":providerId"] },
      },
    });
  input.item.push({
    name: "Models",
    item: [
      {
        name: "delete model",
        request: {
          method: "DELETE",
          url: { path: ["api", "v1", "models", ":modelId"] },
        },
      },
    ],
  });
  input.item.push({ name: "Model roles", item: [] });
  const output = postprocess(input);
  assert.deepEqual(
    output.item.slice(-2).map((folder) => folder.name),
    ["Lifecycle", "Logout"],
  );
  assert.deepEqual(
    output.item.at(-2).item.map((request) => request.name),
    ["delete model", "delete provider"],
  );
  assert.equal(
    output.item.find((folder) => folder.name === "Auth").item.length,
    1,
  );
});

test("asserts successful login and create status instead of accepting unauthorized responses", () => {
  const output = postprocess(structuredClone(raw));
  const login = output.item.find((folder) => folder.name === "Auth").item[0];
  const create = output.item.find((folder) => folder.name === "Providers")
    .item[0];
  assert.match(login.event[0].script.exec.join("\n"), /expected status.*200/);
  assert.match(create.event[0].script.exec.join("\n"), /expected status.*201/);
});

test("uses the cookie jar publicly and an explicit service token internally", () => {
  const input = structuredClone(raw);
  input.auth = { type: "apikey", apikey: [{ key: "in", value: "header" }] };
  input.item.push({
    name: "Internal",
    item: [
      {
        name: "resolve",
        request: {
          method: "GET",
          url: { path: ["internal", "models", "resolve"] },
        },
      },
    ],
  });
  const output = postprocess(input);
  assert.deepEqual(output.auth, { type: "noauth" });
  assert.deepEqual(
    output.item.find((folder) => folder.name === "Providers").item[0].request
      .auth,
    { type: "noauth" },
  );
  assert.deepEqual(
    output.item.find((folder) => folder.name === "Internal").item[0].request
      .auth,
    {
      type: "bearer",
      bearer: [{ key: "token", value: "{{serviceToken}}", type: "string" }],
    },
  );
});
