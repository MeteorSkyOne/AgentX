// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { conversationSkills, sendMessage } from "../api/client";
import type { Agent, ConversationAgentSkills } from "../api/types";
import { Composer, isAcceptedDraftAttachment, selectDraftAttachmentFiles } from "./Composer";

vi.mock("../api/client", () => ({
  conversationSkills: vi.fn(),
  sendMessage: vi.fn()
}));

afterEach(() => {
  cleanup();
  vi.mocked(conversationSkills).mockReset();
  vi.mocked(sendMessage).mockReset();
});

describe("selectDraftAttachmentFiles", () => {
  it("keeps valid files when another dropped file is unsupported", () => {
    const image = new File([new Uint8Array([1, 2, 3])], "screen.png", { type: "image/png" });
    const binary = new File([new Uint8Array([1, 2, 3])], "tool.exe", {
      type: "application/octet-stream"
    });

    const result = selectDraftAttachmentFiles([], [image, binary]);

    expect(result.accepted).toEqual([image]);
    expect(result.rejected).toEqual(["tool.exe is not a supported attachment type"]);
  });

  it("accepts source files selected by extension when the browser omits MIME type", () => {
    const source = new File(["package main"], "main.go", { type: "" });

    expect(isAcceptedDraftAttachment(source)).toBe(true);
  });

  it("rejects files beyond the per-message attachment limit without dropping available slots", () => {
    const existing = Array.from({ length: 4 }, (_, index) =>
      new File(["x"], `existing-${index}.txt`, { type: "text/plain" })
    );
    const first = new File(["x"], "first.txt", { type: "text/plain" });
    const second = new File(["x"], "second.txt", { type: "text/plain" });

    const result = selectDraftAttachmentFiles(existing, [first, second]);

    expect(result.accepted).toEqual([first]);
    expect(result.rejected).toEqual(["second.txt exceeds the 5 file limit"]);
  });
});

describe("Composer slash command autocomplete", () => {
  it("does not load dynamic skills until slash autocomplete is active", async () => {
    const textarea = renderComposer({ skills: [] });

    await nextTick();
    expect(conversationSkills).not.toHaveBeenCalled();

    setTextareaValue(textarea, "hello", 5);
    await nextTick();
    expect(conversationSkills).not.toHaveBeenCalled();

    setTextareaValue(textarea, "/", 1);
    expect(await screen.findByText("/skills")).toBeTruthy();
    expect(conversationSkills).toHaveBeenCalledTimes(1);
  });

  it("shows the static /stop command", () => {
    const textarea = renderComposer({ skills: [] });

    setTextareaValue(textarea, "/st", 3);

    expect(screen.getByText("/stop")).toBeTruthy();
    expect(screen.queryByText("/skill")).toBeNull();
  });

  it("shows dynamic skills after the API data loads", async () => {
    const textarea = renderComposer({
      skills: [
        {
          agent_id: "agt_codex",
          agent_handle: "codex",
          agent_name: "Codex",
          skills: [
            {
              name: "reviewer",
              display_name: "Reviewer",
              description: "Review code",
              conflicts_with_builtin: false
            }
          ]
        }
      ]
    });

    setTextareaValue(textarea, "/rev", 4);

    expect(await screen.findByText("/reviewer")).toBeTruthy();
    expect(screen.getByText("Review code")).toBeTruthy();
  });

  it("inserts a multi-agent skill with the target name and sends the handle", async () => {
    const textarea = renderComposer({
      mentionAgents: [agent("agt_codex", "Codex", "codex"), agent("agt_claude", "Claude", "claude")],
      skills: [
        {
          agent_id: "agt_codex",
          agent_handle: "codex",
          agent_name: "Codex",
          skills: [
            {
              name: "reviewer",
              display_name: "Reviewer",
              description: "Review code",
              conflicts_with_builtin: false
            }
          ]
        }
      ]
    });

    setTextareaValue(textarea, "/rev please", 4);
    fireEvent.mouseDown((await screen.findByText("/reviewer")).closest("button")!);

    expect(textarea.value).toBe("/reviewer @Codex please");

    fireEvent.submit(textarea.closest("form")!);
    await waitFor(() => {
      expect(sendMessage).toHaveBeenCalledWith(
        "channel",
        "chn_1",
        "/reviewer @codex please",
        expect.objectContaining({ files: [] })
      );
    });
  });

  it("inserts a single-agent skill without a target handle", async () => {
    const textarea = renderComposer({
      skills: [
        {
          agent_id: "agt_codex",
          agent_handle: "codex",
          agent_name: "Codex",
          skills: [
            {
              name: "reviewer",
              display_name: "Reviewer",
              description: "Review code",
              conflicts_with_builtin: false
            }
          ]
        }
      ]
    });

    setTextareaValue(textarea, "/rev please", 4);
    fireEvent.mouseDown((await screen.findByText("/reviewer")).closest("button")!);

    expect(textarea.value).toBe("/reviewer please");
  });
});

describe("Composer mention autocomplete", () => {
  it("shows handles in suggestions while inserting names into the composer", async () => {
    const textarea = renderComposer({
      skills: [],
      mentionAgents: [agent("agt_codex", "Codex", "codex")]
    });

    setTextareaValue(textarea, "@co", 3);
    const handleHint = await screen.findByText("@codex");
    fireEvent.mouseDown(handleHint.closest("button")!);

    expect(textarea.value).toBe("@Codex ");
  });

  it("sends composer mention names as handles", async () => {
    const textarea = renderComposer({
      skills: [],
      mentionAgents: [agent("agt_codex", "Codex", "codex")]
    });

    setTextareaValue(textarea, "please ask @Codex", 17);
    fireEvent.submit(textarea.closest("form")!);

    await waitFor(() => {
      expect(sendMessage).toHaveBeenCalledWith(
        "channel",
        "chn_1",
        "please ask @codex",
        expect.objectContaining({ files: [] })
      );
    });
  });
});

function renderComposer({
  skills,
  mentionAgents = [agent("agt_codex", "Codex", "codex")]
}: {
  skills: ConversationAgentSkills[];
  mentionAgents?: Pick<Agent, "id" | "name" | "handle" | "kind" | "bot_user_id">[];
}): HTMLTextAreaElement {
  vi.mocked(conversationSkills).mockResolvedValue(skills);
  vi.mocked(sendMessage).mockResolvedValue({
    id: "msg_1",
    organization_id: "org_1",
    conversation_type: "channel",
    conversation_id: "chn_1",
    sender_type: "user",
    sender_id: "usr_1",
    kind: "text",
    body: "",
    created_at: "2026-05-02T00:00:00Z"
  });
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false }
    }
  });
  render(
    <QueryClientProvider client={queryClient}>
      <Composer
        conversation={{ type: "channel", id: "chn_1", label: "#general" }}
        mentionAgents={mentionAgents}
        onSent={() => undefined}
      />
    </QueryClientProvider>
  );
  return screen.getByLabelText("Message") as HTMLTextAreaElement;
}

function setTextareaValue(textarea: HTMLTextAreaElement, value: string, caret: number) {
  fireEvent.change(textarea, { target: { value } });
  textarea.setSelectionRange(caret, caret);
  fireEvent.keyUp(textarea);
}

function nextTick() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function agent(
  id: string,
  name: string,
  handle: string
): Pick<Agent, "id" | "name" | "handle" | "kind" | "bot_user_id"> {
  return {
    id,
    name,
    handle,
    kind: "codex",
    bot_user_id: `bot_${id}`
  };
}
