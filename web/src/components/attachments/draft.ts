export interface DraftAttachment {
  id: string;
  file: File;
  previewURL?: string;
}

export const maxDraftAttachments = 5;
export const maxDraftAttachmentBytes = 10 * 1024 * 1024;
export const maxDraftAttachmentTotalBytes = 25 * 1024 * 1024;

const imageDraftContentTypes = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);

export function isImageFile(file: File): boolean {
  return imageDraftContentTypes.has(file.type.toLowerCase());
}

export function createDraftAttachment(file: File): DraftAttachment {
  const previewURL = isImageFile(file) ? URL.createObjectURL(file) : undefined;
  return {
    id: draftAttachmentID(),
    file,
    previewURL
  };
}

function draftAttachmentID(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function selectDraftAttachmentFiles(
  existingFiles: File[],
  incomingFiles: File[]
): { accepted: File[]; rejected: string[] } {
  const accepted: File[] = [];
  const rejected: string[] = [];
  let totalBytes = existingFiles.reduce((sum, file) => sum + file.size, 0);
  let remainingSlots = Math.max(0, maxDraftAttachments - existingFiles.length);

  for (const file of incomingFiles) {
    const name = file.name || "Attachment";
    if (remainingSlots <= 0) {
      rejected.push(`${name} exceeds the ${maxDraftAttachments} file limit`);
      continue;
    }
    if (file.size === 0) {
      rejected.push(`${name} is empty`);
      continue;
    }
    if (file.size > maxDraftAttachmentBytes) {
      rejected.push(`${name} exceeds 10 MiB`);
      continue;
    }
    if (totalBytes + file.size > maxDraftAttachmentTotalBytes) {
      rejected.push(`${name} would exceed 25 MiB total`);
      continue;
    }
    accepted.push(file);
    remainingSlots -= 1;
    totalBytes += file.size;
  }

  return { accepted, rejected };
}

export function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${Math.ceil(bytes / 1024)} KB`;
  }
  return `${bytes} B`;
}

export function revokeAttachmentPreviews(attachments: DraftAttachment[]) {
  for (const attachment of attachments) {
    if (attachment.previewURL) {
      URL.revokeObjectURL(attachment.previewURL);
    }
  }
}
