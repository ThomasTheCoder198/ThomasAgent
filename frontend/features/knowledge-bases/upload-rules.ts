/** Mirrors contracts/kb-upload.json (ragx enforces it; upload-rules.test.ts fails when they drift). */
export const UPLOAD_MAX_FILE_BYTES = 52_428_800;
export const UPLOAD_EXTENSIONS = [
  ".pdf",
  ".docx",
  ".xlsx",
  ".csv",
  ".pptx",
  ".md",
  ".txt",
  ".html",
  ".htm",
  ".png",
  ".jpg",
  ".jpeg",
] as const;

export type UploadProblem = "type" | "size" | "empty";

const EXTENSION = /\.[^./\\]+$/;

export function checkUpload(file: { name: string; size: number }): UploadProblem | null {
  const extension = EXTENSION.exec(file.name)?.[0]?.toLowerCase();
  if (!extension || !(UPLOAD_EXTENSIONS as readonly string[]).includes(extension)) return "type";
  if (file.size === 0) return "empty";
  if (file.size > UPLOAD_MAX_FILE_BYTES) return "size";
  return null;
}

export const UPLOAD_ACCEPT = UPLOAD_EXTENSIONS.join(",");
