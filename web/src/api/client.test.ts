// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { clearToken, sendMessage, setToken, type UploadProgress } from "./client";

type Listener = (event: ProgressEvent) => void;

class ListenerBag {
  private listeners = new Map<string, Listener[]>();

  addEventListener(type: string, listener: Listener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  emit(type: string, event: Partial<ProgressEvent> = {}) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event as ProgressEvent);
    }
  }
}

class FakeXHR extends ListenerBag {
  static last: FakeXHR | null = null;

  method = "";
  url = "";
  headers: Record<string, string> = {};
  body: FormData | null = null;
  status = 200;
  statusText = "OK";
  responseText = "";
  upload = new ListenerBag();

  constructor() {
    super();
    FakeXHR.last = this;
  }

  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(key: string, value: string) {
    this.headers[key] = value;
  }

  send(body: FormData) {
    this.body = body;
  }
}

const originalXHR = globalThis.XMLHttpRequest;

beforeEach(() => {
  FakeXHR.last = null;
  globalThis.XMLHttpRequest = FakeXHR as unknown as typeof XMLHttpRequest;
});

afterEach(() => {
  globalThis.XMLHttpRequest = originalXHR;
  clearToken();
});

function attachment(): File {
  return new File(["hello attachment"], "notes.txt", { type: "text/plain" });
}

describe("sendMessage with attachments", () => {
  it("reports upload progress and resolves with the created message", async () => {
    setToken("session-token");
    const seen: UploadProgress[] = [];

    const pending = sendMessage("channel", "chn_1", "with files", {
      files: [attachment()],
      onUploadProgress: (progress) => seen.push(progress)
    });

    const xhr = FakeXHR.last;
    expect(xhr).not.toBeNull();
    expect(xhr?.method).toBe("POST");
    expect(xhr?.url).toBe("/api/conversations/channel/chn_1/messages");
    expect(xhr?.headers.Authorization).toBe("Bearer session-token");
    expect(xhr?.body).toBeInstanceOf(FormData);

    xhr?.upload.emit("progress", { lengthComputable: true, loaded: 250, total: 1000 });
    xhr!.responseText = JSON.stringify({ id: "msg_1" });
    xhr?.emit("load");

    await expect(pending).resolves.toEqual({ id: "msg_1" });
    // Starts at zero, tracks the wire, then completes even though the last
    // progress event stopped short.
    expect(seen).toEqual([
      { loaded: 0, total: 0, percent: 0 },
      { loaded: 250, total: 1000, percent: 25 },
      { loaded: 1000, total: 1000, percent: 100 }
    ]);
  });

  it("leaves progress indeterminate when the browser cannot compute the total", async () => {
    const seen: UploadProgress[] = [];
    const pending = sendMessage("channel", "chn_1", "with files", {
      files: [attachment()],
      onUploadProgress: (progress) => seen.push(progress)
    });

    const xhr = FakeXHR.last;
    xhr?.upload.emit("progress", { lengthComputable: false, loaded: 250, total: 0 });
    xhr!.responseText = "{}";
    xhr?.emit("load");
    await pending;

    expect(seen[1]).toEqual({ loaded: 250, total: 0, percent: 0 });
  });

  it("rejects with the server error message", async () => {
    const pending = sendMessage("channel", "chn_1", "with files", { files: [attachment()] });

    const xhr = FakeXHR.last;
    xhr!.status = 400;
    xhr!.statusText = "Bad Request";
    xhr!.responseText = JSON.stringify({ error: "malformed multipart form" });
    xhr?.emit("load");

    await expect(pending).rejects.toThrow("malformed multipart form");
  });

  it("rejects when the upload fails at the transport level", async () => {
    const pending = sendMessage("channel", "chn_1", "with files", { files: [attachment()] });
    FakeXHR.last?.emit("error");
    await expect(pending).rejects.toThrow("Upload failed");
  });
});
