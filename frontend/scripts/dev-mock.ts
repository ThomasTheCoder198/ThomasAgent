/**
 * `npm run dev:mock`: starts the fake core and `next dev` pointed at it, so the whole UI runs
 * without the Go stack. Switching to the real core is only `WEB_CORE_URL` (see README).
 */
import { spawn, type ChildProcess } from "node:child_process";

import { mockConfig } from "../mock-core/config.ts";

const coreUrl = `http://localhost:${mockConfig.port}`;
const isWindows = process.platform === "win32";

function run(command: string, args: string[], env: Record<string, string>): ChildProcess {
  return spawn(command, args, { stdio: "inherit", env: { ...process.env, ...env }, shell: isWindows });
}

const children = [
  run(process.execPath, ["mock-core/server.ts"], {}),
  run("npx", ["next", "dev"], { WEB_CORE_URL: coreUrl }),
];

function shutdown(code: number) {
  for (const child of children) child.kill();
  process.exit(code);
}

for (const child of children) child.on("exit", (code) => shutdown(code ?? 1));
process.on("SIGINT", () => shutdown(0));
process.on("SIGTERM", () => shutdown(0));
