import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";

test("CI generation check rejects a newly generated untracked file", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "m0-2-ci-"));
  t.after(() => {
    const resolved = fs.realpathSync(root);
    assert.ok(resolved.startsWith(path.join(fs.realpathSync(os.tmpdir()), "m0-2-ci-")));
    fs.rmSync(resolved, { recursive: true, force: true });
  });
  const workspace = path.join(root, "repo");
  fs.mkdirSync(workspace);
  const init = spawnSync("git", ["init", "--quiet", workspace], { encoding: "utf8" });
  assert.equal(init.status, 0, init.stderr);
  const workflow = fs.readFileSync(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");
  const lines = workflow.split(/\r?\n/);
  const step = lines.findIndex((line) => line.includes("name: Generated code is up to date"));
  assert.ok(step >= 0);
  const start = lines.findIndex((line, index) => index > step && line.trim() === "run: |");
  const body = [];
  for (let index = start + 1; index < lines.length && lines[index].startsWith("          "); index++) {
    body.push(lines[index].slice(10));
  }
  assert.ok(body.length > 0);
  const script = path.join(root, "check.sh");
  fs.writeFileSync(script, [
    "set -euo pipefail",
    "task() { mkdir -p backend-core/internal/generated; printf '{}' > backend-core/internal/generated/new.json; }",
    ...body,
  ].join("\n"));
  const bash = process.platform === "win32" ? path.join(process.env.ProgramFiles, "Git/bin/bash.exe") : "bash";
  const checked = spawnSync(bash, [script.replaceAll("\\", "/")], { cwd: workspace, encoding: "utf8" });
  assert.equal(checked.error, undefined);
  assert.ok(fs.existsSync(path.join(workspace, "backend-core/internal/generated/new.json")), checked.stderr);
  assert.equal(checked.status, 1, "CI must fail when generation creates an untracked output");
});
