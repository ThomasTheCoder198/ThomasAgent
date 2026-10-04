import { fileURLToPath, pathToFileURL } from "node:url";

import newman from "newman";

const DEFAULT_BASE_URL = "http://localhost:8080";
const PUBLIC_FOLDERS = ["Health", "Auth", "Providers", "Models", "Model roles"];
const FINAL_FOLDERS = ["Lifecycle", "Logout"];
const FAILED_EXIT_CODE = 1;

export function buildRunOptions(environment) {
  const email = environment.CORE_AUTH_OWNER_EMAIL;
  const password = environment.CORE_AUTH_OWNER_PASSWORD;
  if (!email)
    throw new Error(
      "CORE_AUTH_OWNER_EMAIL is required for the API collection.",
    );
  if (!password)
    throw new Error(
      "CORE_AUTH_OWNER_PASSWORD is required for the API collection.",
    );
  const token = environment.CORE_SERVICE_TOKEN;
  return {
    collection: fileURLToPath(
      new URL("./thomasagent.postman_collection.json", import.meta.url),
    ),
    environment: fileURLToPath(
      new URL("./local.postman_environment.json", import.meta.url),
    ),
    envVar: [
      {
        key: "baseUrl",
        value: environment.CORE_API_TEST_BASE_URL || DEFAULT_BASE_URL,
      },
      { key: "ownerEmail", value: email },
      { key: "ownerPassword", value: password },
      ...(token ? [{ key: "serviceToken", value: token }] : []),
    ],
    folder: [
      ...PUBLIC_FOLDERS,
      ...(token ? ["Internal"] : []),
      ...FINAL_FOLDERS,
    ],
    // A summary-only report keeps request/response secrets out of reporter output.
    reporters: [],
  };
}

export function runCollection(
  environment,
  execute = (options, callback) => newman.run(options, callback),
) {
  const options = buildRunOptions(environment);
  return new Promise((resolve, reject) => {
    execute(options, (error, summary) =>
      error ? reject(error) : resolve(summary),
    );
  });
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    const summary = await runCollection(process.env);
    const { stats, failures } = summary.run;
    console.log(
      `Newman: ${stats.requests.total} requests, ${stats.assertions.total} assertions, ${failures.length} failures.`,
    );
    for (const failure of failures) {
      console.error(
        `Failed check: ${failure.error.test || failure.error.name} (${failure.source?.name || "collection"}).`,
      );
    }
    if (failures.length) process.exitCode = FAILED_EXIT_CODE;
  } catch {
    console.error(
      "API collection could not complete; verify the server, owner credentials, and installed contract tooling.",
    );
    process.exitCode = FAILED_EXIT_CODE;
  }
}
