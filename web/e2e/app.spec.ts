import { expect, test } from "@playwright/test";
import type { Message } from "../src/api/types";
import {
  createChannelViaAPI,
  e2ePassword,
  e2eSetupToken,
  e2eUsername,
  expectMonacoEditorPosition,
  expectMonacoEditorText,
  firstEnabledAgent,
  firstProject,
  readWorkspaceFile,
  setMonacoEditorValue,
  signIn,
  uniqueHandle,
  uniqueName,
  writeWorkspaceFile,
} from "./helpers";

const displayName = "E2E User";

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("validates login, restores the session, and logs back in", async ({ page }) => {
  await page.evaluate(() => localStorage.setItem("agentx.session_token", "invalid-token"));
  await page.goto("/");
  await expect(page.getByLabel("Username")).toBeVisible();

  if (await page.getByLabel("Setup token").isVisible().catch(() => false)) {
    await expect(page.getByRole("button", { name: "Set up admin" })).toBeDisabled();
    await page.getByLabel("Setup token").fill("wrong-token");
    await page.getByLabel("Username").fill(e2eUsername);
    await page.getByLabel("Display name").fill(displayName);
    await page.getByLabel("Password", { exact: true }).fill(e2ePassword);
    await page.getByLabel("Confirm password").fill(e2ePassword);
    await page.getByRole("button", { name: "Set up admin" }).click();
    await expect(page.getByText("unauthorized")).toBeVisible();

    await page.getByLabel("Setup token").fill(e2eSetupToken);
    await page.getByLabel("Password", { exact: true }).fill("qwe123");
    await page.getByLabel("Confirm password").fill("qwe123");
    await page.getByRole("button", { name: "Set up admin" }).click();
    await expect(page.getByText("password must be at least 12 bytes")).toBeVisible();

    await page.getByLabel("Password", { exact: true }).fill(e2ePassword);
    await page.getByLabel("Confirm password").fill(e2ePassword);
    await page.getByRole("button", { name: "Set up admin" }).click();
  } else {
    await expect(page.getByRole("button", { name: "Log in" })).toBeDisabled();
    await page.getByLabel("Username").fill(e2eUsername);
    await page.getByLabel("Password", { exact: true }).fill("wrong-password");
    await page.getByRole("button", { name: "Log in" }).click();
    await expect(page.getByText("unauthorized")).toBeVisible();

    await page.getByLabel("Password", { exact: true }).fill(e2ePassword);
    await page.getByRole("button", { name: "Log in" }).click();
  }

  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Default" })).toBeVisible();
  await expect(page.getByText(displayName)).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();

  await page.getByRole("button", { name: "Log out" }).click();
  await expect(page.getByLabel("Username")).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toHaveCount(0);
  expect(await page.evaluate(() => localStorage.getItem("agentx.session_token"))).toBeNull();

  await page.getByLabel("Username").fill(e2eUsername);
  await page.getByLabel("Password", { exact: true }).fill(e2ePassword);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
});

test("sends messages from the composer and receives agent output", async ({ page }) => {
  await signIn(page);
  const channel = await createChannelViaAPI(page, test.info(), { prefix: "composer" });
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await page.getByRole("button", { name: channel.name, exact: true }).click();

  await expect(page.getByText("No messages yet")).toBeVisible();

  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill("button ping");
  await page.getByRole("button", { name: "Send" }).click();

  let messages = page.getByLabel("Messages");
  await expect(messages.getByText("button ping", { exact: true })).toBeVisible();
  await expect(messages.getByText("Echo: button ping", { exact: true })).toBeVisible();
  await expect(composer).toHaveValue("");

  await composer.fill("enter ping");
  await composer.press("Enter");

  messages = page.getByLabel("Messages");
  await expect(messages.getByText("enter ping", { exact: true })).toBeVisible();
  await expect(messages.getByText("Echo: enter ping", { exact: true })).toBeVisible();

  await composer.fill("line one");
  await composer.press("Shift+Enter");
  await expect(composer).toHaveValue("line one\n");
  await page.keyboard.insertText("line two");
  await page.getByRole("button", { name: "Send" }).click();

  messages = page.getByLabel("Messages");
  await expect(messages.getByText("line one line two", { exact: true })).toBeVisible();
  await expect(messages.getByText("Echo: line one line two", { exact: true })).toBeVisible();
  await expect(composer).toHaveValue("");
});

test("uploads and downloads a text attachment from the composer", async ({ page }) => {
  await signIn(page);
  const channel = await createChannelViaAPI(page, test.info(), { prefix: "attachments" });
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await page.getByRole("button", { name: channel.name, exact: true }).click();

  const attachmentName = "attachment-e2e-notes.txt";
  const attachmentBody = "hello from an e2e attachment";
  await page.locator('input[type="file"]').setInputFiles({
    name: attachmentName,
    mimeType: "text/plain",
    buffer: Buffer.from(attachmentBody),
  });
  await expect(page.getByText(attachmentName, { exact: true })).toBeVisible();

  const messageResponse = page.waitForResponse((response) => {
    return (
      response.request().method() === "POST" &&
      response.url().includes(`/api/conversations/channel/${channel.id}/messages`) &&
      response.status() === 200
    );
  });
  await page.getByRole("textbox", { name: "Message" }).fill("please read the attachment");
  await page.getByRole("button", { name: "Send" }).click();

  const sent = (await (await messageResponse).json()) as Message;
  expect(sent.attachments).toHaveLength(1);
  const [attachment] = sent.attachments ?? [];
  expect(attachment).toBeDefined();
  if (!attachment) throw new Error("missing attachment in sent message");
  expect(attachment.filename).toBe(attachmentName);
  expect(attachment.kind).toBe("text");

  const messages = page.getByLabel("Messages");
  await expect(messages.getByText("please read the attachment", { exact: true })).toBeVisible();
  await expect(messages.getByText(attachmentName, { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Preview attachment" }).click();
  const previewDialog = page.getByRole("dialog");
  await expect(previewDialog.getByText(attachmentName, { exact: true })).toBeVisible();
  await expect(previewDialog.getByTestId("workspace-file-editor")).toBeVisible();
  await expectMonacoEditorText(previewDialog, attachmentBody);

  const content = await page.evaluate(async (attachmentID) => {
    const token = localStorage.getItem("agentx.session_token");
    if (!token) throw new Error("missing session token");
    const response = await fetch(`/api/attachments/${encodeURIComponent(attachmentID)}/content`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    return {
      status: response.status,
      text: await response.text(),
      contentType: response.headers.get("Content-Type"),
      contentSecurityPolicy: response.headers.get("Content-Security-Policy"),
      cacheControl: response.headers.get("Cache-Control"),
    };
  }, attachment.id);

  expect(content.status).toBe(200);
  expect(content.text).toBe(attachmentBody);
  expect(content.contentType).toContain("text/plain");
  expect(content.contentSecurityPolicy).toBe("default-src 'none'; sandbox");
  expect(content.cacheControl).toBe("private, no-store");
});

test("opens image attachments in an in-app preview dialog", async ({ page }) => {
  await signIn(page);
  const channel = await createChannelViaAPI(page, test.info(), { prefix: "image attachments" });
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await page.getByRole("button", { name: channel.name, exact: true }).click();

  const imageName = "attachment-e2e-image.png";
  await page.locator('input[type="file"]').setInputFiles({
    name: imageName,
    mimeType: "image/png",
    buffer: Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
      "base64"
    ),
  });
  await expect(page.getByText(imageName, { exact: true })).toBeVisible();

  await page.getByRole("textbox", { name: "Message" }).fill("please inspect the image");
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  await expect(messages.getByText("please inspect the image", { exact: true })).toBeVisible();
  await messages.getByRole("button", { name: `Preview ${imageName}` }).click();

  const previewDialog = page.getByRole("dialog");
  await expect(previewDialog.getByText(imageName, { exact: true })).toBeVisible();
  const previewImage = previewDialog.getByRole("img", { name: imageName });
  await expect(previewImage).toBeVisible();
  await expect(previewImage).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");

  await previewDialog.getByTestId("image-preview-canvas").hover();
  await page.mouse.wheel(0, -600);
  await expect.poll(() => previewImage.evaluate((node) => getComputedStyle(node).transform)).not.toBe(
    "matrix(1, 0, 0, 1, 0, 0)"
  );
  const zoomedTransform = await previewImage.evaluate((node) => getComputedStyle(node).transform);

  const canvasBox = await previewDialog.getByTestId("image-preview-canvas").boundingBox();
  if (!canvasBox) throw new Error("missing image preview canvas box");
  await page.mouse.move(canvasBox.x + canvasBox.width / 2, canvasBox.y + canvasBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(canvasBox.x + canvasBox.width / 2 + 80, canvasBox.y + canvasBox.height / 2 + 40);
  await page.mouse.up();
  await expect.poll(() => previewImage.evaluate((node) => getComputedStyle(node).transform)).not.toBe(
    zoomedTransform
  );
});

test("opens and closes the bound agent details panel", async ({ page }) => {
  await signIn(page);

  await page.getByLabel("Agents").getByRole("button", { name: /Fake Agent/ }).click();
  await expect(page.getByLabel("Agent details")).toBeVisible();
  await page.getByLabel("Agent details").getByRole("button", { name: "Close" }).click();
  await expect(page.getByLabel("Agent details")).toHaveCount(0);

  await page.getByRole("button", { name: "Agent settings" }).click();
  const agentPanel = page.getByLabel("Agent details");
  await expect(agentPanel.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
  await expect(agentPanel.getByText("Fake runtime")).toBeVisible();
  await expect(agentPanel.getByText("fake-echo")).toBeVisible();
  await expect(agentPanel.getByText("Channel", { exact: true })).toBeVisible();
  await expect(agentPanel.locator(".workspace-path").getByText(/fake-default/)).toBeVisible();
  await expect(agentPanel.getByText("empty")).toBeVisible();

  await page.getByRole("button", { name: "Agent settings" }).click();
  await expect(page.getByLabel("Agent details")).toHaveCount(0);
});

test("agent settings accepts a custom effort value", async ({ page }, testInfo) => {
  await signIn(page);
  const customEffort = uniqueHandle(testInfo, "custom_effort");

  await page.getByRole("button", { name: "Agent settings" }).click();
  const panel = page.getByLabel("Agent details");
  await panel.getByLabel("Agent effort").fill(customEffort);
  await panel.getByRole("button", { name: "Save" }).click();
  await expect(panel.getByText("Saved")).toBeVisible();
  await expect(panel.getByLabel("Agent effort")).toHaveValue(customEffort);

  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await page.getByRole("button", { name: "Agent settings" }).click();
  await expect(
    page.getByLabel("Agent details").getByLabel("Agent effort")
  ).toHaveValue(customEffort);
});

test("keeps an empty project workspace path draft while editing", async ({ page }) => {
  await signIn(page);

  await page.getByRole("button", { name: "Project settings" }).click();
  const dialog = page.getByRole("dialog", { name: "Project settings" });
  const workspacePath = dialog.getByLabel("Workspace path");
  await expect(workspacePath).not.toHaveValue("");

  await workspacePath.fill("");
  await expect(workspacePath).toHaveValue("");
  await expect(dialog.getByRole("button", { name: "Save" })).toBeDisabled();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});

test("desktop project files opens project editor and persists changes", async ({ page }) => {
  await signIn(page);
  const project = await firstProject(page);
  await writeWorkspaceFile(page, project.workspace_id, "src/app.ts", "export const value = 1;");
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();

  await page.getByRole("button", { name: "Project files" }).click();
  const projectTree = page.getByRole("tree", { name: "Project files" });
  await expect(projectTree).toBeVisible();
  await expect(page.getByRole("button", { name: "Create channel" })).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Message" })).toHaveCount(0);

  await projectTree.getByRole("treeitem", { name: "app.ts" }).click();
  const editor = page.getByTestId("project-file-editor-pane");
  await expect(editor).toBeVisible();
  await expect(editor.getByText("src/app.ts")).toBeVisible();
  await expectMonacoEditorText(editor, "export const value = 1;");

  await setMonacoEditorValue(page, editor, "export const value = 2;");
  await editor.getByRole("button", { name: "Save file" }).click();
  await expect(editor.getByText("Saved")).toBeVisible();
  await expect.poll(() => readWorkspaceFile(page, project.workspace_id, "src/app.ts")).toBe("export const value = 2;");

  await setMonacoEditorValue(page, editor, "");
  await editor.getByRole("button", { name: "Open" }).click();
  await expect(editor.getByText("Loaded")).toBeVisible();
  await expectMonacoEditorText(editor, "export const value = 2;");

  await page.getByRole("button", { name: "Project files" }).click();
  await expect(page.getByRole("button", { name: "Create channel" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
});

test("desktop message file links open project files at the requested position", async ({ page }, testInfo) => {
  await signIn(page);
  const project = await firstProject(page);
  const channel = await createChannelViaAPI(page, testInfo, { prefix: "file links" });
  const filePath = "web/src/App.tsx";
  await writeWorkspaceFile(
    page,
    project.workspace_id,
    filePath,
    [
      "line one",
      "line two",
      "line three",
      "const selectedColumn = 7;",
      "line five",
    ].join("\n")
  );

  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await page.getByRole("button", { name: channel.name, exact: true }).click();

  const targetLabel = `${filePath}:4:7`;
  await page.getByRole("textbox", { name: "Message" }).fill(`Please inspect ${targetLabel}.`);
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  const messageRow = messages
    .locator("[data-message-id]")
    .filter({ hasText: "You" })
    .filter({ hasText: targetLabel })
    .last();
  await expect(messageRow).toBeVisible();
  await messageRow.getByRole("button", { name: `Open ${targetLabel}` }).click();

  const projectTree = page.getByRole("tree", { name: "Project files" });
  await expect(projectTree).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toHaveCount(0);

  const editor = page.getByTestId("project-file-editor-pane");
  await expect(editor).toBeVisible();
  await expect(editor.getByText(filePath)).toBeVisible();
  await expectMonacoEditorText(editor, "const selectedColumn = 7;");
  await expectMonacoEditorPosition(page, editor, { lineNumber: 4, column: 7 });
});

test("desktop missing message file links show a prominent editor error", async ({ page }, testInfo) => {
  await signIn(page);
  const project = await firstProject(page);
  const channel = await createChannelViaAPI(page, testInfo, { prefix: "missing file links" });
  await writeWorkspaceFile(page, project.workspace_id, "src/existing.ts", "const oldContent = true;");

  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await page.getByRole("button", { name: "Project files" }).click();
  const editor = page.getByTestId("project-file-editor-pane");
  await editor.getByLabel("File path").fill("src/existing.ts");
  await editor.getByRole("button", { name: "Open" }).click();
  await expectMonacoEditorText(editor, "const oldContent = true;");
  await page.getByRole("button", { name: "Project files" }).click();

  await page.getByRole("button", { name: channel.name, exact: true }).click();
  const missingLabel = "src/not-found.ts:7:2";
  await page.getByRole("textbox", { name: "Message" }).fill(`Please inspect ${missingLabel}.`);
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  const messageRow = messages
    .locator("[data-message-id]")
    .filter({ hasText: "You" })
    .filter({ hasText: missingLabel })
    .last();
  await expect(messageRow).toBeVisible();
  await messageRow.getByRole("button", { name: `Open ${missingLabel}` }).click();

  await expect(editor).toBeVisible();
  const loadError = editor.getByTestId("workspace-file-load-error");
  await expect(loadError).toBeVisible();
  await expect(loadError.getByText("File not found", { exact: true })).toBeVisible();
  await expect(loadError.getByText("src/not-found.ts", { exact: true })).toBeVisible();
  await expect(loadError.getByText("file not found", { exact: true })).toBeVisible();
  await expect(loadError.getByRole("button", { name: "Retry" })).toBeVisible();
  await expect(editor.getByText("const oldContent = true;")).toHaveCount(0);
});

test("agent files tab remains scoped to the agent workspace", async ({ page }) => {
  await signIn(page);
  const project = await firstProject(page);
  const agent = await firstEnabledAgent(page);
  await writeWorkspaceFile(page, project.workspace_id, "memory.md", "project memory");
  await writeWorkspaceFile(page, agent.config_workspace_id, "memory.md", "agent memory");

  await page.getByRole("button", { name: "Project files" }).click();
  const projectTree = page.getByRole("tree", { name: "Project files" });
  await expect(projectTree).toBeVisible();
  const projectEditor = page.getByTestId("project-file-editor-pane");
  await projectEditor.getByLabel("File path").fill("memory.md");
  await projectEditor.getByRole("button", { name: "Open" }).click();
  await expectMonacoEditorText(projectEditor, "project memory");
  await page.getByRole("button", { name: "Project files" }).click();

  await page.getByRole("button", { name: "Agent settings" }).click();
  const panel = page.getByLabel("Agent details");
  await panel.getByRole("tab", { name: /Files/ }).click();
  await expect(panel.getByLabel("Workspace", { exact: true })).toHaveCount(0);
  await panel.getByLabel("File path").fill("memory.md");
  await panel.getByRole("button", { name: "Open" }).click();
  await expectMonacoEditorText(panel, "agent memory");
  await expect(panel.getByTestId("workspace-file-editor")).not.toContainText("project memory");
});

test("manages projects, channel agents, threads, and workspace files", async ({ page }) => {
  await signIn(page);
  const projectName = uniqueName(test.info(), "Ops");
  const channelName = uniqueName(test.info(), "lab");
  const agentName = uniqueName(test.info(), "Agent Two");
  const agentHandle = uniqueHandle(test.info(), "agent_two");
  const threadChannelName = uniqueName(test.info(), "posts");
  const threadTitle = uniqueName(test.info(), "First post");
  const threadBody = `thread hello ${uniqueHandle(test.info(), "body")}`;

  await page.getByRole("button", { name: "Create project" }).click();
  const projectModal = page.getByRole("dialog");
  await projectModal.getByLabel("Project name").fill(projectName);
  await projectModal.getByRole("button", { name: "Save" }).click();
  await expect(page.getByRole("button", { name: projectName, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Create channel" }).click();
  const channelModal = page.getByRole("dialog");
  await channelModal.getByLabel("Channel name").fill(channelName);
  await channelModal.getByLabel("Channel type").selectOption("text");
  await channelModal.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByRole("button", { name: channelName, exact: true })).toBeVisible();

  // Create a new agent via modal
  await page.getByRole("button", { name: "Create agent" }).click();
  const agentModal = page.getByRole("dialog");
  await agentModal.getByLabel("New agent name").fill(agentName);
  await agentModal.getByLabel("New agent handle").fill(agentHandle);
  await agentModal.getByRole("button", { name: "Create", exact: true }).click();

  // Open agent panel and verify the new agent exists
  await page.getByRole("button", { name: "Agent settings" }).click();
  const panel = page.getByLabel("Agent details");
  await expect(panel.getByLabel("Agent", { exact: true })).toContainText(agentName);

  // Use the standalone Members panel to manage channel bindings.
  await panel.getByRole("button", { name: "Close" }).click();
  await page.getByRole("button", { name: "Members" }).click();
  const members = page.getByLabel("Channel members");
  await expect(members).toBeVisible();
  await members.locator(".picker-row").filter({ hasText: "Fake Agent" }).getByRole("checkbox").check();
  await members.locator(".picker-row").filter({ hasText: agentName }).getByRole("checkbox").check();
  await members.getByRole("button", { name: "Save" }).click();
  await expect(members.locator(".picker-row").filter({ hasText: agentName }).getByRole("checkbox")).toBeChecked();
  await members.getByRole("button", { name: "Close members" }).click();

  // Test multi-agent messaging.
  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill("multi ping");
  await page.getByRole("button", { name: "Send" }).click();
  const messages = page.getByLabel("Messages");
  await expect(messages.getByText("Echo: multi ping", { exact: true })).toHaveCount(2);

  await composer.fill(`@${agentHandle.slice(0, 3)}`);
  await page.getByRole("button", { name: new RegExp(`${escapeRegExp(agentName)}.*@${escapeRegExp(agentHandle)}`) }).click();
  await expect(composer).toHaveValue(`@${agentHandle} `);
  await page.keyboard.insertText("directed");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(messages.getByText(`Echo: @${agentHandle} directed`, { exact: true })).toHaveCount(1);
  await expect(messages.locator(`[data-mention="${agentHandle}"]`).first()).toBeVisible();

  // Open panel, switch to Files tab for workspace file management
  await page.getByRole("button", { name: "Agent settings" }).click();
  await panel.getByLabel("Agent", { exact: true }).selectOption({ label: `${agentName} (@${agentHandle})` });
  await panel.getByRole("tab", { name: /Files/ }).click();
  await panel.getByLabel("File path").fill("memory.md");
  await setMonacoEditorValue(page, panel, "memory from e2e");
  await panel.getByRole("button", { name: "Save file" }).click();
  await setMonacoEditorValue(page, panel, "");
  await panel.getByRole("button", { name: "Open" }).click();
  await expectMonacoEditorText(panel, "memory from e2e");
  await page.getByRole("button", { name: "Agent settings" }).click();

  // Create a thread/forum channel
  await page.getByRole("button", { name: "Create channel" }).click();
  await page.getByLabel("Channel name").fill(threadChannelName);
  await page.getByLabel("Channel type").selectOption("thread");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByLabel("Threads")).toBeVisible();

  await page.getByLabel("Post title").fill(threadTitle);
  await page.getByLabel("Post body").fill(threadBody);
  await page.getByRole("button", { name: "Create post" }).click();
  await expect(page.getByRole("heading", { name: threadTitle })).toBeVisible();
  await expect(page.getByLabel("Messages").getByText(threadBody, { exact: true })).toBeVisible();
});

test("confirms before deleting an agent", async ({ page }) => {
  await signIn(page);
  const disposableHandle = uniqueHandle(test.info(), "disposable");
  const disposableName = disposableHandle.replace(/_/g, " ");

  await page.getByRole("button", { name: "Create agent" }).click();
  const agentModal = page.getByRole("dialog");
  await agentModal.getByLabel("New agent name").fill(disposableName);
  await agentModal.getByLabel("New agent handle").fill(disposableHandle);
  await agentModal.getByRole("button", { name: "Create", exact: true }).click();

  await page.getByRole("button", { name: "Agent settings" }).click();
  const panel = page.getByLabel("Agent details");
  await panel.getByLabel("Agent", { exact: true }).selectOption({ label: `${disposableName} (@${disposableHandle})` });
  await panel.getByRole("button", { name: "Delete agent" }).click();

  const confirm = page.getByRole("dialog", { name: "Delete agent?" });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Cancel" }).click();
  await expect(confirm).toHaveCount(0);
  await expect(panel.getByLabel("Agent", { exact: true })).toContainText(disposableName);

  await panel.getByRole("button", { name: "Delete agent" }).click();
  await page.getByRole("dialog", { name: "Delete agent?" }).getByRole("button", { name: "Delete" }).click();
  await expect(panel.getByText("Deleted")).toBeVisible();

  await panel.getByRole("button", { name: "Close" }).click();
  await page.getByRole("button", { name: "Members" }).click();
  await expect(page.getByLabel("Channel members").getByText(disposableName)).toHaveCount(0);
  await page.getByLabel("Channel members").getByRole("button", { name: "Close members" }).click();

  const cleanupHandle = uniqueHandle(test.info(), "cleanup");
  const cleanupName = cleanupHandle.replace(/_/g, " ");
  await page.getByRole("button", { name: "Create agent" }).click();
  await page.getByRole("dialog").getByLabel("New agent name").fill(cleanupName);
  await page.getByRole("dialog").getByLabel("New agent handle").fill(cleanupHandle);
  await page.getByRole("dialog").getByRole("button", { name: "Create", exact: true }).click();

  await page.getByRole("button", { name: "Agent settings" }).click();
  const cleanupPanel = page.getByLabel("Agent details");
  await cleanupPanel.getByLabel("Agent", { exact: true }).selectOption({ label: `${cleanupName} (@${cleanupHandle})` });
  await cleanupPanel.getByRole("button", { name: "Delete agent" }).click();
  await page.getByRole("dialog", { name: "Delete agent?" }).getByRole("button", { name: "Delete" }).click();
  await expect(cleanupPanel.getByText("Deleted")).toBeVisible();
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
