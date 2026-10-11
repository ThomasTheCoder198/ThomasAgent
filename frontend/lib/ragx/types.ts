/** Ragx's API types, generated from contracts/openapi/ragx.v1.yaml by `task gen` (rule 3: never hand-written). */
import type { components } from "./api.gen";

type Schemas = components["schemas"];

export type KnowledgeBase = Schemas["KnowledgeBase"];
export type KnowledgeBaseWrite = Schemas["KnowledgeBaseWrite"];
export type KbFile = Schemas["KbFile"];
export type FileStatus = KbFile["status"];
export type UploadRequest = Schemas["UploadRequest"];
export type UploadTicket = Schemas["UploadTicket"];
export type DownloadUrl = Schemas["DownloadUrl"];
export type FilePage = Schemas["PageMeta"]["page"];
