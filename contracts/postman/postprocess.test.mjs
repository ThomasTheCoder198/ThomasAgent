import assert from "node:assert/strict";
import { test } from "node:test";

import { postprocess } from "./postprocess.mjs";

const raw = {
  info: { name: "ThomasAgent Core API" },
  item: [
    {
      name: "LLM providers",
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
  const create = out.item.find((f) => f.name === "LLM providers").item[0];
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
  for (const key of ["baseUrl", "csrfToken", "providerId", "modelId", "orgId"])
    assert.ok(names.includes(key), key);
});

test("keeps required query examples while disabling optional generated filters", () => {
  const input = structuredClone(raw);
  input.item.push({
    name: "LLM models",
    item: [
      {
        name: "list models",
        request: {
          method: "GET",
          url: {
            path: ["api", "v1", "models"],
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
  const query = output.item.find((folder) => folder.name === "LLM models")
    .item[0].request.url.query;
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
    .find((folder) => folder.name === "LLM providers")
    .item.push({
      name: "delete provider",
      request: {
        method: "DELETE",
        url: { path: ["api", "v1", "providers", ":providerId"] },
      },
    });
  input.item.push({
    name: "LLM models",
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
  input.item.push({ name: "LLM model roles", item: [] });
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
  const create = output.item.find((folder) => folder.name === "LLM providers")
    .item[0];
  assert.match(login.event[0].script.exec.join("\n"), /expected status.*200/);
  assert.match(create.event[0].script.exec.join("\n"), /expected status.*201/);
});

test("uses the cookie jar for every request: no collection or request auth header", () => {
  const input = structuredClone(raw);
  input.auth = { type: "apikey", apikey: [{ key: "in", value: "header" }] };
  const output = postprocess(input);
  assert.deepEqual(output.auth, { type: "noauth" });
  assert.deepEqual(
    output.item.find((folder) => folder.name === "LLM providers").item[0].request
      .auth,
    { type: "noauth" },
  );
  assert.ok(!output.item.some((folder) => folder.name === "Internal"));
  assert.ok(!JSON.stringify(output).includes("serviceToken"));
});

test("runs the create-organization request first and expects the synchronous provisioning statuses", () => {
  const request = (name, method, path) => ({
    name,
    request: { method, url: { host: ["{{baseUrl}}"], path } },
  });
  const input = structuredClone(raw);
  input.item.push({
    name: "Organizations",
    item: [
      request("list", "GET", ["api", "v1", "orgs"]),
      request("provision", "POST", ["api", "v1", "orgs", ":orgId", "provision"]),
      request("create", "POST", ["api", "v1", "orgs"]),
    ],
  });
  const output = postprocess(input);
  const folder = output.item.find((item) => item.name === "Organizations");
  assert.deepEqual(
    folder.item.map((item) => item.name),
    ["create", "list", "provision"],
  );
  const script = (item) => item.event[0].script.exec.join("\n");
  assert.match(script(folder.item[0]), /expected status.*201/);
  assert.match(script(folder.item[0]), /orgId/);
  assert.match(script(folder.item[2]), /expected status.*200/);
});

test("orders the Apps folder right after Organizations, with no expected statuses", () => {
  const request = (name, method, path) => ({
    name,
    request: { method, url: { host: ["{{baseUrl}}"], path } },
  });
  const input = structuredClone(raw);
  input.item.push(
    {
      name: "Apps",
      item: [
        request("proxy get", "GET", ["api", "v1", "orgs", ":orgId", "apps", "ragx", ":path"]),
      ],
    },
    { name: "Organizations", item: [request("create", "POST", ["api", "v1", "orgs"])] },
  );
  const output = postprocess(input);
  const names = output.item.map((folder) => folder.name);
  assert.equal(names.indexOf("Apps"), names.indexOf("Organizations") + 1);
  const apps = output.item.find((folder) => folder.name === "Apps");
  assert.ok(!JSON.stringify(apps).includes("expected status"));
});

test("orders the Conversations folder after Agents and declares conversationId", () => {
  const request = (name, method, path) => ({
    name,
    request: { method, url: { host: ["{{baseUrl}}"], path } },
  });
  const input = structuredClone(raw);
  input.item.push(
    { name: "Conversations", item: [request("list", "GET", ["api", "v1", "orgs", ":orgId", "conversations"])] },
    { name: "Agents", item: [request("list agents", "GET", ["api", "v1", "orgs", ":orgId", "agents"])] },
    { name: "Apps", item: [] },
  );
  const output = postprocess(input);
  const names = output.item.map((folder) => folder.name);
  assert.equal(names.indexOf("Conversations"), names.indexOf("Agents") + 1);
  assert.ok(names.indexOf("Conversations") < names.indexOf("Apps"));
  assert.ok(output.variable.some((v) => v.key === "conversationId" && v.value === ""));
});

test("runs the platform role clear in Lifecycle after the in-use guards and accepts its 204", () => {
  const request = (name, method, path) => ({
    name,
    request: { method, url: { host: ["{{baseUrl}}"], path } },
  });
  const input = structuredClone(raw);
  input.item.push({
    name: "LLM model roles",
    item: [request("clear role", "DELETE", ["api", "v1", "model-roles", ":role"])],
  });
  input.item.push({
    name: "LLM models",
    item: [request("delete model", "DELETE", ["api", "v1", "models", ":modelId"])],
  });
  const output = postprocess(input);
  const lifecycle = output.item.find((folder) => folder.name === "Lifecycle");
  assert.deepEqual(
    lifecycle.item.map((item) => item.name),
    ["delete model", "clear role"],
  );
  assert.match(lifecycle.item[1].event[0].script.exec.join("\n"), /expected status.*204/);
  assert.match(output.event.find((e) => e.listen === "test").script.exec.join("\n"), /code === 204\) return/);
});

test("runs the org model-role requests after the organization is created", () => {
  const request = (name, method, path) => ({
    name,
    request: { method, url: { host: ["{{baseUrl}}"], path } },
  });
  const input = structuredClone(raw);
  input.item.push(
    {
      name: "LLM model roles",
      item: [request("list org roles", "GET", ["api", "v1", "orgs", ":orgId", "model-roles"])],
    },
    { name: "Organizations", item: [request("create", "POST", ["api", "v1", "orgs"])] },
  );
  const output = postprocess(input);
  const organizations = output.item.find((folder) => folder.name === "Organizations");
  assert.deepEqual(
    organizations.item.map((item) => item.name),
    ["create", "list org roles"],
  );
  assert.equal(output.item.find((folder) => folder.name === "LLM model roles").item.length, 0);
});
