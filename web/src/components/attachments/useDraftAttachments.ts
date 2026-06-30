import { useCallback, useEffect, useRef, useState } from "react";
import type { ChangeEvent, ClipboardEvent, DragEvent } from "react";
import {
  createDraftAttachment,
  revokeAttachmentPreviews,
  selectDraftAttachmentFiles,
  type DraftAttachment
} from "./draft";

/**
 * useDraftAttachments encapsulates the file-attachment draft state shared by the
 * message composer and the thread/post creation form: file selection limits,
 * image previews, paste/drag-and-drop handling, and cleanup of object URLs.
 *
 * Pass an `onError` callback to surface rejection messages (e.g. files that are
 * too large) to the host component's error UI.
 */
export function useDraftAttachments(onError?: (message: string | null) => void) {
  const [attachments, setAttachments] = useState<DraftAttachment[]>([]);
  const [draggingFiles, setDraggingFiles] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const attachmentsRef = useRef<DraftAttachment[]>([]);
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  useEffect(() => {
    attachmentsRef.current = attachments;
  }, [attachments]);

  useEffect(() => {
    return () => {
      revokeAttachmentPreviews(attachmentsRef.current);
    };
  }, []);

  const addFiles = useCallback((files: File[]) => {
    if (files.length === 0) return;
    const selection = selectDraftAttachmentFiles(
      attachmentsRef.current.map((item) => item.file),
      files
    );
    if (selection.rejected.length > 0) {
      onErrorRef.current?.(selection.rejected.slice(0, 3).join("; "));
    } else {
      onErrorRef.current?.(null);
    }
    if (selection.accepted.length > 0) {
      setAttachments((current) => [...current, ...selection.accepted.map(createDraftAttachment)]);
    }
  }, []);

  const removeAttachment = useCallback((attachmentID: string) => {
    setAttachments((current) => {
      const removed = current.find((item) => item.id === attachmentID);
      if (removed?.previewURL) {
        URL.revokeObjectURL(removed.previewURL);
      }
      return current.filter((item) => item.id !== attachmentID);
    });
  }, []);

  const clearAttachments = useCallback((items: DraftAttachment[], revokePreviews = true) => {
    if (revokePreviews) {
      revokeAttachmentPreviews(items);
    }
    setAttachments([]);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }, []);

  const handleFileInputChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      addFiles(Array.from(event.currentTarget.files ?? []));
      event.currentTarget.value = "";
    },
    [addFiles]
  );

  const handlePaste = useCallback(
    (event: ClipboardEvent<Element>) => {
      const files = Array.from(event.clipboardData.files ?? []);
      if (files.length === 0) return;
      event.preventDefault();
      addFiles(files);
    },
    [addFiles]
  );

  const handleDragOver = useCallback((event: DragEvent<Element>) => {
    if (!hasDraggedFiles(event)) return;
    event.preventDefault();
    setDraggingFiles(true);
  }, []);

  const handleDragLeave = useCallback((event: DragEvent<Element>) => {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setDraggingFiles(false);
    }
  }, []);

  const handleDrop = useCallback(
    (event: DragEvent<Element>) => {
      if (!hasDraggedFiles(event)) return;
      event.preventDefault();
      setDraggingFiles(false);
      addFiles(Array.from(event.dataTransfer.files ?? []));
    },
    [addFiles]
  );

  const openFilePicker = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  return {
    attachments,
    setAttachments,
    draggingFiles,
    fileInputRef,
    addFiles,
    removeAttachment,
    clearAttachments,
    handleFileInputChange,
    handlePaste,
    dragHandlers: {
      onDragOver: handleDragOver,
      onDragLeave: handleDragLeave,
      onDrop: handleDrop
    },
    openFilePicker
  };
}

function hasDraggedFiles(event: DragEvent<Element>): boolean {
  return Array.from(event.dataTransfer.types).includes("Files");
}
