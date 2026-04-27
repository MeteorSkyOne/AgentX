import { expect, test, type Page, type TestInfo } from "@playwright/test";
import {
  expectMonacoEditorText,
  firstProject,
  readWorkspaceFile,
  seedDenseNavigation,
  setLightTheme,
  setMonacoEditorValue,
  uniqueName,
  writeWorkspaceFile,
} from "./helpers";

const adminToken = "e2e-token";
const displayName = "Mobile E2E User";

async function signInMobile(page: Page) {
  await page.goto("/");

  await page.getByLabel("Admin token").fill(adminToken);
  await page.getByLabel("Display name").fill(displayName);
  await page.getByRole("button", { name: "Enter" }).click();

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
