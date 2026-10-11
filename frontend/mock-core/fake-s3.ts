import { createHash } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";
import { Readable } from "node:stream";

import { mockConfig } from "./config.ts";
import { HTTP_NO_CONTENT } from "./http.ts";

/**
 * A fake of MinIO's presigned POST and GET for the `kb-raw` bucket. Public and outside /api/v1, like the real
 * thing: no session, no CSRF. It checks the policy's `content-length-range` and answers CORS for the web origin.
 */
const HTTP_OK = 200;
const HTTP_BAD_REQUEST = 400;
const HTTP_NOT_FOUND = 404;
const KEY_FIELD = "key";
const POLICY_FIELD = "policy";
const FILE_FIELD = "file";

export type StoredObject = { bytes: Buffer; name: string; sha256: string };

const objects = new Map<string, StoredObject>();

export function resetObjects() {
  objects.clear();
}

export function putObject(key: string, bytes: Buffer, name: string): StoredObject {
  const stored = { bytes, name, sha256: createHash("sha256").update(bytes).digest("hex") };
  objects.set(key, stored);
  return stored;
}

export const getObject = (key: string) => objects.get(key);
export const deleteObject = (key: string) => objects.delete(key);

const cors = { "Access-Control-Allow-Origin": mockConfig.webOrigin };

function xmlError(res: ServerResponse, status: number, code: string) {
  res.writeHead(status, { "Content-Type": "application/xml", ...cors });
  res.end(
    `<?xml version="1.0" encoding="UTF-8"?><Error><Code>${code}</Code><Message>${code}</Message></Error>`,
  );
}

export function preflight(_req: IncomingMessage, res: ServerResponse) {
  res.writeHead(HTTP_NO_CONTENT, {
    ...cors,
    "Access-Control-Allow-Methods": "GET, POST",
    "Access-Control-Allow-Headers": "*",
  });
  res.end();
}

type Condition = [string, ...unknown[]];

function sizeRange(policy: string): [number, number] | undefined {
  try {
    const { conditions } = JSON.parse(Buffer.from(policy, "base64").toString("utf8")) as {
      conditions: Condition[];
    };
    const range = conditions.find((c) => c[0] === "content-length-range");
    return range ? [Number(range[1]), Number(range[2])] : undefined;
  } catch {
    return undefined;
  }
}

async function readForm(req: IncomingMessage): Promise<FormData> {
  const headers = new Headers();
  for (const [name, value] of Object.entries(req.headers))
    if (typeof value === "string") headers.set(name, value);
  const request = new Request(new URL(req.url ?? "/", mockConfig.publicUrl), {
    method: req.method,
    headers,
    body: Readable.toWeb(req) as ReadableStream,
    duplex: "half",
  } as RequestInit);
  return request.formData();
}

/** The presigned POST: policy fields first, the file last; the object is stored under the policy's `key`. */
export async function postObject(req: IncomingMessage, res: ServerResponse) {
  let form: FormData;
  try {
    form = await readForm(req);
  } catch {
    return xmlError(res, HTTP_BAD_REQUEST, "MalformedPOSTRequest");
  }
  const key = form.get(KEY_FIELD);
  const policy = form.get(POLICY_FIELD);
  const file = form.get(FILE_FIELD);
  if (typeof key !== "string" || typeof policy !== "string" || !(file instanceof File)) {
    return xmlError(res, HTTP_BAD_REQUEST, "MalformedPOSTRequest");
  }
  const range = sizeRange(policy);
  if (range && file.size > range[1]) return xmlError(res, HTTP_BAD_REQUEST, "EntityTooLarge");
  if (range && file.size < range[0]) return xmlError(res, HTTP_BAD_REQUEST, "EntityTooSmall");
  putObject(key, Buffer.from(await file.arrayBuffer()), file.name);
  res.writeHead(HTTP_NO_CONTENT, cors);
  res.end();
}

/** The presigned GET: always an attachment with a generic type, exactly what the real URL forces. */
export function getObjectResponse(_req: IncomingMessage, res: ServerResponse, key: string) {
  const stored = getObject(decodeURIComponent(key));
  if (!stored) return xmlError(res, HTTP_NOT_FOUND, "NoSuchKey");
  res.writeHead(HTTP_OK, {
    ...cors,
    "Content-Type": "application/octet-stream",
    "Content-Disposition": `attachment; filename*=UTF-8''${encodeURIComponent(stored.name)}`,
  });
  res.end(stored.bytes);
}
