import { readFileSync, writeFileSync } from "node:fs";

const FOLDER_ORDER = [
  "Health",
  "Auth",
  "LLM providers",
  "LLM models",
  "LLM model roles",
  "Organizations",
  "Agents",
  "Conversations",
  "Apps",
  "Lifecycle",
  "Logout",
];
// Requests are matched by method + path because the generator names them from summaries/operationIds inconsistently.
const LOGIN_ROUTE = "POST /api/v1/auth/login";
const LOGOUT_ROUTE = "POST /api/v1/auth/logout";
// Clearing the platform role comes last: the two in-use guards need the role still assigned.
const DELETE_ROUTES = [
  "DELETE /api/v1/models/{modelId}",
  "DELETE /api/v1/providers/{providerId}",
  "DELETE /api/v1/model-roles/{role}",
];
const SAVE_ID_BY_ROUTE = {
  "POST /api/v1/providers": "providerId",
  "POST /api/v1/models": "modelId",
  "POST /api/v1/orgs": "orgId",
  "POST /api/v1/orgs/{orgId}/agents": "agentId",
};
const EXPECTED_STATUS_BY_ROUTE = {
  "GET /healthz": 200,
  "GET /readyz": 200,
  "POST /api/v1/auth/login": 200,
  "GET /api/v1/auth/me": 200,
  "POST /api/v1/auth/logout": 200,
  "GET /api/v1/providers": 200,
  "POST /api/v1/providers": 201,
  "PATCH /api/v1/providers/{providerId}": 200,
  "POST /api/v1/providers/{providerId}/test": 400,
  "POST /api/v1/providers/{providerId}/sync": 400,
  "DELETE /api/v1/providers/{providerId}": 409,
  "GET /api/v1/models": 200,
  "POST /api/v1/models": 201,
  "DELETE /api/v1/models/{modelId}": 409,
  "GET /api/v1/model-roles": 200,
  "PUT /api/v1/model-roles/{role}": 200,
  "DELETE /api/v1/model-roles/{role}": 204,
  "GET /api/v1/orgs": 200,
  "POST /api/v1/orgs": 201,
  "GET /api/v1/orgs/{orgId}": 200,
  "PATCH /api/v1/orgs/{orgId}": 200,
  "POST /api/v1/orgs/{orgId}/provision": 200,
  "GET /api/v1/orgs/{orgId}/agents": 200,
  "POST /api/v1/orgs/{orgId}/agents": 201,
  "GET /api/v1/orgs/{orgId}/agents/{agentId}": 200,
  "PATCH /api/v1/orgs/{orgId}/agents/{agentId}": 200,
  "DELETE /api/v1/orgs/{orgId}/agents/{agentId}": 204,
};
const ORGANIZATIONS_FOLDER = "Organizations";
// Org model-role requests read {{orgId}}, so they run in the Organizations folder after the create request.
const ORG_MODEL_ROLES_PREFIX = "/api/v1/orgs/{orgId}/model-roles";
const CREATE_ORGANIZATION_ROUTE = "POST /api/v1/orgs";
const JSON_INDENT = 2;

const csrfPreRequest = [
  "const method = pm.request.method.toUpperCase();",
  "if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && pm.collectionVariables.get('csrfToken')) {",
  "  pm.request.headers.upsert({ key: 'X-CSRF-Token', value: pm.collectionVariables.get('csrfToken') });",
  "}",
];

const responseFormatTests = [
  "pm.test('no 5xx', () => pm.expect(pm.response.code).to.be.below(500));",
  "pm.test('response format', () => {",
  "  if (pm.response.code === 204) return;",
  "  const body = pm.response.json();",
  "  if (pm.response.code < 400) {",
  "    pm.expect(body).to.have.property('data');",
  "    pm.expect(body.meta.requestId).to.be.a('string').and.not.empty;",
  "  } else {",
  "    pm.expect(body.error.code).to.be.a('string').and.not.empty;",
  "    pm.expect(body.error.message).to.be.a('string').and.not.empty;",
  "  }",
  "});",
];

const script = (listen, exec) => ({
  listen,
  script: { type: "text/javascript", exec },
});

// openapi-to-postmanv2 emits random ids and randomly generated saved responses on every run;
// dropping them keeps `task gen` byte-identical. Saved responses are documentation only, tests do not use them.
function stripIds(node) {
  if (Array.isArray(node)) return node.forEach(stripIds);
  if (node && typeof node === "object") {
    delete node.id;
    delete node._postman_id;
    if (node.request) {
      node.response = [];
      // Optional query params without examples get random values; ship them empty and disabled.
      for (const q of node.request.url?.query ?? []) {
        const description =
          typeof q.description === "string"
            ? q.description
            : (q.description?.content ?? "");
        if (description.includes("(Required)")) {
          q.disabled = false;
          continue;
        }
        q.value = "";
        q.disabled = true;
      }
    }
    Object.values(node).forEach(stripIds);
  }
}

function routeOf(req) {
  const url = req.request.url;
  const path =
    typeof url === "string"
      ? new URL(url.replace("{{baseUrl}}", "http://x")).pathname
      : `/${(url.path ?? []).join("/")}`;
  return `${req.request.method.toUpperCase()} ${path.replace(/:([^/]+)/g, "{$1}")}`;
}

function eachRequest(items, fn) {
  for (const item of items) {
    if (item.item) eachRequest(item.item, fn);
    else fn(item);
  }
}

function orderLifecycle(collection) {
  const lifecycle = [];
  const logout = [];
  for (const folder of collection.item) {
    if (!folder.item) continue;
    folder.item = folder.item.filter((request) => {
      if (request.item) return true;
      const route = routeOf(request);
      if (DELETE_ROUTES.includes(route)) lifecycle.push(request);
      else if (route === LOGOUT_ROUTE) logout.push(request);
      else return true;
      return false;
    });
  }
  lifecycle.sort(
    (left, right) =>
      DELETE_ROUTES.indexOf(routeOf(left)) -
      DELETE_ROUTES.indexOf(routeOf(right)),
  );
  if (lifecycle.length)
    collection.item.push({ name: "Lifecycle", item: lifecycle });
  if (logout.length) collection.item.push({ name: "Logout", item: logout });
}

// The other organization requests read {{orgId}}, which only the create request saves.
function orderFolderFirst(collection, folderName, route) {
  const folder = collection.item.find((item) => item.name === folderName);
  if (!folder?.item) return;
  folder.item.sort(
    (left, right) =>
      Number(routeOf(right) === route) - Number(routeOf(left) === route),
  );
}

function moveOrgModelRolesToOrganizations(collection) {
  const target = collection.item.find((item) => item.name === ORGANIZATIONS_FOLDER);
  if (!target?.item) return;
  for (const folder of collection.item) {
    if (!folder.item || folder === target) continue;
    folder.item = folder.item.filter((request) => {
      if (request.item || !routeOf(request).split(" ")[1].startsWith(ORG_MODEL_ROLES_PREFIX)) return true;
      target.item.push(request);
      return false;
    });
  }
}

export function postprocess(collection) {
  stripIds(collection);
  // Auth must remain live until role assignment and the in-use deletion guards have run.
  orderLifecycle(collection);
  moveOrgModelRolesToOrganizations(collection);
  orderFolderFirst(
    collection,
    ORGANIZATIONS_FOLDER,
    CREATE_ORGANIZATION_ROUTE,
  );
  collection.item.sort(
    (a, b) => FOLDER_ORDER.indexOf(a.name) - FOLDER_ORDER.indexOf(b.name),
  );
  // The converter maps cookie apiKey schemes to headers; Newman already persists Set-Cookie in its jar.
  collection.auth = { type: "noauth" };
  collection.event = [
    script("prerequest", csrfPreRequest),
    script("test", responseFormatTests),
  ];
  eachRequest(collection.item, (req) => {
    const route = routeOf(req);
    req.request.auth = { type: "noauth" };
    const tests = [];
    const expectedStatus = EXPECTED_STATUS_BY_ROUTE[route];
    if (expectedStatus)
      tests.push(
        `pm.test('expected status', () => pm.expect(pm.response.code).to.equal(${expectedStatus}));`,
      );
    if (route === LOGIN_ROUTE) {
      tests.push(
        "if (pm.response.code === 200) pm.collectionVariables.set('csrfToken', pm.response.json().data.csrfToken);",
      );
    }
    const idVar = SAVE_ID_BY_ROUTE[route];
    if (idVar) {
      tests.push(
        `if (pm.response.code === 201) pm.collectionVariables.set('${idVar}', pm.response.json().data.id);`,
      );
    }
    if (route === "POST /api/v1/orgs/{orgId}/provision") {
      tests.push("if (pm.response.code === 200) pm.collectionVariables.set('kbId', pm.response.json().data.defaultKnowledgeBaseId);");
    }
    if (tests.length) req.event = [script("test", tests)];
  });
  const keep = (collection.variable ?? []).filter(
    (v) =>
      !["baseUrl", "csrfToken", "providerId", "modelId", "orgId", "agentId", "conversationId", "kbId"].includes(
        v.key,
      ),
  );
  collection.variable = [
    ...keep,
    { key: "baseUrl", value: "http://localhost:8080" },
    { key: "csrfToken", value: "" },
    { key: "providerId", value: "" },
    { key: "modelId", value: "" },
    { key: "orgId", value: "" },
    { key: "agentId", value: "" },
    { key: "conversationId", value: "" },
    { key: "kbId", value: "" },
  ];
  return collection;
}

if (process.argv[2] && process.argv[3]) {
  const raw = JSON.parse(readFileSync(process.argv[2], "utf8"));
  writeFileSync(
    process.argv[3],
    JSON.stringify(postprocess(raw), null, JSON_INDENT) + "\n",
  );
}
