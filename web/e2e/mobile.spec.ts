import { expect, test, type Page, type TestInfo } from "@playwright/test";
import {
  expectMonacoEditorPosition,
  expectMonacoEditorText,
  firstProject,
  readWorkspaceFile,
  seedDenseNavigation,
  setLightTheme,
  setMonacoEditorValue,
  signIn,
  uniqueName,
  writeWorkspaceFile,
} from "./helpers";

const displayName = "Mobile E2E User";

async function signInMobile(page: Page) {
  await signIn(page, displayName);

  await expect(page.getByTestId("mobile-shell")).toBeVisible();
  await expect(page.getByTestId("desktop-shell")).toBeHidden();
  await expect(page.getByRole("button", { name: "Navigation" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await expect(page.getByTestId("mobile-shell").getByRole("heading").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function openNavigation(page: Page) {
  await page.getByRole("button", { name: "Navigation" }).click();
  const nav = page.getByRole("dialog", { name: "Navigation" });
  await expect(nav).toBeVisible();
  return nav;
}

async function createChannel(page: Page, testInfo: TestInfo, type: "text" | "thread") {
  const nav = await openNavigation(page);
  await nav.getByRole("button", { name: "Create channel" }).click();

  const channelName = uniqueName(testInfo, type);
  const channelModal = page.getByRole("dialog", { name: "Create channel" });
  await channelModal.getByLabel("Channel name").fill(channelName);
  await channelModal.getByLabel("Channel type").selectOption(type);
  await channelModal.getByRole("button", { name: "Create", exact: true }).click();
  await expect(channelModal).toHaveCount(0);
  await expectNoHorizontalOverflow(page);
  return channelName;
}

async function expectNoHorizontalOverflow(page: Page) {
  const widths = await page.evaluate(() => ({
    body: document.body.scrollWidth,
    doc: document.documentElement.scrollWidth,
    viewport: document.documentElement.clientWidth,
  }));
  expect(Math.max(widths.body, widths.doc)).toBeLessThanOrEqual(widths.viewport + 1);
}

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("mobile navigation, messaging, and side panels are usable", async ({ page }, testInfo) => {
  await signInMobile(page);

  const nav = await openNavigation(page);
  await expect(nav.getByRole("button", { name: /general/i })).toBeVisible();
  await nav.getByRole("button", { name: "Close navigation" }).click();
  await expect(nav).toHaveCount(0);

  const composer = page.getByRole("textbox", { name: "Message" });
  const messageText = uniqueName(testInfo, "mobile ping");
  await composer.fill(messageText);
  await page.getByRole("button", { name: "Send" }).click();
  const messages = page.getByLabel("Messages");
  await expect(messages.getByText(messageText, { exact: true })).toBeVisible();
  await expect(messages.getByText(`Echo: ${messageText}`, { exact: true }).first()).toBeVisible();
  await expect(composer).toHaveValue("");
  await expectNoHorizontalOverflow(page);

  await createChannel(page, testInfo, "text");

  await page.getByRole("button", { name: "Members" }).click();
  const members = page.getByLabel("Channel members");
  await expect(members).toBeVisible();
  await expect(members.getByText("Fake Agent")).toBeVisible();
  await members.getByRole("button", { name: "Close members" }).click();
  await expect(members).toHaveCount(0);

  await page.getByRole("button", { name: "Agent settings" }).click();
  const agentPanel = page.getByLabel("Agent details");
  await expect(agentPanel).toBeVisible();
  await expect(agentPanel.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
  await agentPanel.getByRole("button", { name: "Close" }).click();
  await expect(agentPanel).toHaveCount(0);
  await expectNoHorizontalOverflow(page);
});

test("mobile chat keeps composer visible and message pane scrollable with very long messages", async ({ page }, testInfo) => {
  await signInMobile(page);

  const marker = uniqueName(testInfo, "mobile long scroll");
  const messageText = Array.from({ length: 90 }, (_, index) => {
    const line = String(index + 1).padStart(2, "0");
    return `${marker} line ${line} ${"message content ".repeat(8)}`;
  }).join("\n");

  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill(messageText);
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  const viewport = messages.locator('[data-slot="scroll-area-viewport"]');
  await expect(messages).toContainText(`Echo: ${marker}`);
  await expect(composer).toHaveValue("");
  await expect(composer).toBeInViewport({ ratio: 1 });

  await expect
    .poll(() => viewport.evaluate((node) => node.scrollHeight - node.clientHeight))
    .toBeGreaterThan(0);

  await viewport.evaluate((node) => {
    node.scrollTop = 0;
  });
  await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBe(0);
  await expect(composer).toBeInViewport({ ratio: 1 });

  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect
    .poll(() =>
      viewport.evaluate((node) => {
        const maxScrollTop = node.scrollHeight - node.clientHeight;
        return maxScrollTop > 0 && node.scrollTop >= maxScrollTop - 2;
      })
    )
    .toBe(true);
  await expect(composer).toBeInViewport({ ratio: 1 });

  const followUp = uniqueName(testInfo, "after long mobile");
  await composer.fill(followUp);
  await page.getByRole("button", { name: "Send" }).click();
  await expect(messages.getByText(followUp, { exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("mobile messages can be copied", async ({ page, context }, testInfo) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await signInMobile(page);

  const messageKey = uniqueName(testInfo, "copy markdown");
  const messageText = [
    messageKey,
    "",
    "Keep **raw markdown** when copied.",
  ].join("\n");

  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill(messageText);
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  const row = messages.locator("[data-message-id]").filter({ hasText: "You" }).filter({ hasText: messageKey }).last();
  await expect(row.getByTestId("message-body")).toBeVisible();
  await row.getByRole("button", { name: "Copy message" }).click();
  await expect(row.getByRole("button", { name: "Message copied" })).toBeVisible();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(messageText);
});

test("mobile messages keep wide markdown content accessible", async ({ page }) => {
  await signInMobile(page);

  const longToken = `mobile-wide-${"0123456789abcdef".repeat(18)}`;
  const messageText = [
    "mobile markdown stress",
    "",
    `Inline code: \`${longToken}\``,
    "",
    "```ts",
    `const value = "${longToken}";`,
    "```",
    "",
    "| field | value |",
    "| --- | --- |",
    `| token | ${longToken} |`,
  ].join("\n");

  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill(messageText);
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  const body = messages.getByTestId("message-body").filter({ hasText: "mobile markdown stress" }).first();
  await expect(body).toBeVisible();

  const bodyMetrics = await body.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    return {
      overflowX: getComputedStyle(node).overflowX,
      right: rect.right,
      viewport: document.documentElement.clientWidth,
    };
  });
  expect(bodyMetrics.overflowX).not.toBe("hidden");
  expect(bodyMetrics.right).toBeLessThanOrEqual(bodyMetrics.viewport + 1);

  const codeBlock = body.getByTestId("code-block").first();
  await expect(codeBlock).toBeVisible();
  const codeMetrics = await codeBlock.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    return {
      clientWidth: node.clientWidth,
      overflowX: getComputedStyle(node).overflowX,
      right: rect.right,
      scrollWidth: node.scrollWidth,
      viewport: document.documentElement.clientWidth,
    };
  });
  expect(codeMetrics.overflowX).toBe("auto");
  expect(codeMetrics.right).toBeLessThanOrEqual(codeMetrics.viewport + 1);
  expect(codeMetrics.scrollWidth).toBeGreaterThan(codeMetrics.clientWidth);

  await codeBlock.evaluate((node) => {
    node.scrollLeft = node.scrollWidth;
  });
  await expect.poll(() => codeBlock.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);

  const table = body.locator("table").first();
  await expect(table).toBeVisible();
  const tableMetrics = await table.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    const firstHeader = node.querySelector("th");
    return {
      firstHeaderWidth: firstHeader?.getBoundingClientRect().width ?? 0,
      right: rect.right,
      viewport: document.documentElement.clientWidth,
    };
  });
  expect(tableMetrics.firstHeaderWidth).toBeGreaterThan(80);
  expect(tableMetrics.right).toBeLessThanOrEqual(tableMetrics.viewport + 1);
  await expectNoHorizontalOverflow(page);
});

test("mobile navigation opens project settings", async ({ page }) => {
  await signInMobile(page);

  const nav = await openNavigation(page);
  await nav.getByRole("button", { name: "Project settings" }).click();

  const dialog = page.getByRole("dialog", { name: "Project settings" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel("Workspace path")).not.toHaveValue("");
  await expect(nav).toHaveCount(0);

  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  await expectNoHorizontalOverflow(page);
});

test("mobile thread channel can create and open a post", async ({ page }, testInfo) => {
  await signInMobile(page);
  await createChannel(page, testInfo, "thread");

  await expect(page.getByLabel("Threads")).toBeVisible();
  const postTitle = uniqueName(testInfo, "Mobile post");
  await page.getByLabel("Post title").fill(postTitle);
  await page.getByLabel("Post body").fill("thread hello from mobile");
  await page.getByRole("button", { name: "Create post" }).click();

  await expect(page.getByRole("heading", { name: postTitle })).toBeVisible();
  await expect(page.getByLabel("Messages").getByText("thread hello from mobile", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Back to posts" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("mobile project files navigates tree editor and back to chat", async ({ page }) => {
  await signInMobile(page);
  const project = await firstProject(page);
  await writeWorkspaceFile(page, project.workspace_id, "docs/mobile.md", "mobile project file");

  await page.getByRole("button", { name: "Project files" }).click();
  await expect(page.getByRole("button", { name: "Navigation" })).toHaveCount(0);
  const projectTree = page.getByRole("tree", { name: "Project files" });
  await expect(projectTree).toBeVisible();
  await projectTree.getByRole("treeitem", { name: "mobile.md" }).click();

  const editor = page.getByTestId("project-file-editor-pane");
  await expect(editor).toBeVisible();
  await expect(page.getByRole("tree", { name: "Project files" })).toHaveCount(0);
  await expectMonacoEditorText(editor, "mobile project file");

  await setMonacoEditorValue(page, editor, "mobile project file edited");
  await editor.getByRole("button", { name: "Save file" }).click();
  await expect(editor.getByText("Saved")).toBeVisible();
  await expect.poll(() => readWorkspaceFile(page, project.workspace_id, "docs/mobile.md")).toBe("mobile project file edited");
  await setMonacoEditorValue(page, editor, "");
  await editor.getByRole("button", { name: "Open" }).click();
  await expectMonacoEditorText(editor, "mobile project file edited");

  await page.getByRole("button", { name: "Back to files" }).click();
  await expect(projectTree).toBeVisible();
  await page.getByRole("button", { name: "Back to chat" }).click();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await expect(page.getByRole("button", { name: "Navigation" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("mobile message file links open the project editor directly", async ({ page }) => {
  await signInMobile(page);
  const project = await firstProject(page);
  const filePath = "docs/mobile-link.ts";
  await writeWorkspaceFile(
    page,
    project.workspace_id,
    filePath,
    [
      "mobile line one",
      "mobile line two",
      "let selectedColumn = 5;",
      "mobile line four",
    ].join("\n")
  );

  const targetLabel = `${filePath}:3:5`;
  await page.getByRole("textbox", { name: "Message" }).fill(`Open ${targetLabel}.`);
  await page.getByRole("button", { name: "Send" }).click();

  const messages = page.getByLabel("Messages");
  const messageRow = messages
    .locator("[data-message-id]")
    .filter({ hasText: "You" })
    .filter({ hasText: targetLabel })
    .last();
  await expect(messageRow).toBeVisible();
  await messageRow.getByRole("button", { name: `Open ${targetLabel}` }).click();

  await expect(page.getByRole("button", { name: "Navigation" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Back to files" })).toBeVisible();
  await expect(page.getByRole("tree", { name: "Project files" })).toHaveCount(0);

  const editor = page.getByTestId("project-file-editor-pane");
  await expect(editor).toBeVisible();
  await expect(editor.getByText(filePath)).toBeVisible();
  await expectMonacoEditorText(editor, "let selectedColumn = 5;");
  await expectMonacoEditorPosition(page, editor, { lineNumber: 3, column: 5 });
  await expectNoHorizontalOverflow(page);
});

test("mobile navigation keeps fixed header and footer with dense light-mode data", async ({ page }, testInfo) => {
  await signInMobile(page);
  await setLightTheme(page);
  await seedDenseNavigation(page, testInfo);
  await page.reload();

  await expect(page.getByTestId("mobile-shell")).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  expect(await page.locator("html").evaluate((node) => node.classList.contains("dark"))).toBe(false);

  const nav = await openNavigation(page);
  const viewport = page.viewportSize();
  const navBox = await nav.boundingBox();
  expect(viewport).not.toBeNull();
  expect(navBox).not.toBeNull();
  expect(navBox!.x).toBeGreaterThanOrEqual(-1);
  expect(navBox!.width).toBeLessThanOrEqual(viewport!.width + 1);

  const scrollViewport = nav.getByTestId("mobile-nav-scroll").locator('[data-slot="scroll-area-viewport"]');
  const metrics = await scrollViewport.evaluate((node) => ({
    clientHeight: node.clientHeight,
    scrollHeight: node.scrollHeight,
  }));
  expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);

  const header = nav.getByTestId("mobile-nav-header");
  const footer = nav.getByTestId("mobile-nav-footer");
  await scrollViewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  const scrolledMetrics = await scrollViewport.evaluate((node) => ({
    clientHeight: node.clientHeight,
    maxScrollTop: node.scrollHeight - node.clientHeight,
    scrollTop: node.scrollTop,
  }));
  expect(scrolledMetrics.scrollTop).toBeGreaterThan(0);
  expect(scrolledMetrics.scrollTop).toBeGreaterThanOrEqual(scrolledMetrics.maxScrollTop - 2);

  await expect(header).toBeInViewport({ ratio: 1 });
  await expect(header.getByRole("button", { name: "Close navigation" })).toBeInViewport({ ratio: 1 });
  await expect(footer).toBeInViewport({ ratio: 1 });
  await expect(footer.getByRole("button", { name: "User settings" })).toBeInViewport({ ratio: 1 });
  await expect(footer.getByRole("button", { name: "Switch to dark mode" })).toBeInViewport({ ratio: 1 });
  await expect(footer.getByRole("button", { name: "Log out" })).toBeInViewport({ ratio: 1 });
  await expectNoHorizontalOverflow(page);
});
