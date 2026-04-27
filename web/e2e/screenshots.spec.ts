import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { firstProject, seedDenseNavigation, setLightTheme, setMonacoEditorValue, signIn, writeWorkspaceFile } from "./helpers";
const currentDir = path.dirname(fileURLToPath(import.meta.url));

test.skip(!process.env.AGENTX_CAPTURE_SCREENSHOTS, "Optional diagnostic screenshots are disabled by default.");

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("captures diagnostic screenshots for AI review", async ({ page }, testInfo) => {
  await page.goto("/");
  await capture(page, testInfo, "01-login");

  await signIn(page, "Screenshot User");
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await clearDefaultChannelMessages(page);
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await capture(page, testInfo, "02-shell-ready");

  const project = await firstProject(page);
  await writeWorkspaceFile(page, project.workspace_id, "docs/screenshot.md", "screenshot project workspace note");
  await page.getByRole("button", { name: "Project files" }).click();
  const projectTree = page.getByRole("tree", { name: "Project files" });
  await expect(projectTree).toBeVisible();
  await capture(page, testInfo, testInfo.project.name.startsWith("mobile") ? "07-mobile-project-files" : "07-project-files");
  await projectTree.getByRole("treeitem", { name: "screenshot.md" }).click();
  await expect(page.getByTestId("project-file-editor-pane")).toBeVisible();
  await capture(page, testInfo, testInfo.project.name.startsWith("mobile") ? "08-mobile-project-file-editor" : "08-project-file-editor");
  if (testInfo.project.name.startsWith("mobile")) {
    await page.getByRole("button", { name: "Back to files" }).click();
    await page.getByRole("button", { name: "Back to chat" }).click();
    await capture(page, testInfo, "09-mobile-restored-chat");
  } else {
    await page.getByRole("button", { name: "Project files" }).click();
    await capture(page, testInfo, "09-restored-chat");
  }

  if (testInfo.project.name.startsWith("mobile")) {
    await page.getByRole("button", { name: "Navigation" }).click();
    await expect(page.getByRole("dialog", { name: "Navigation" })).toBeVisible();
    await capture(page, testInfo, "03-mobile-navigation");
    await page.getByRole("button", { name: "Close navigation" }).click();

    await setLightTheme(page);
    await seedDenseNavigation(page, testInfo);
    await page.reload();
    await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
    await page.getByRole("button", { name: "Navigation" }).click();
    const denseNav = page.getByRole("dialog", { name: "Navigation" });
    await expect(denseNav.getByTestId("mobile-nav-footer")).toBeInViewport({ ratio: 1 });
    await capture(page, testInfo, "03b-mobile-navigation-dense-light");
    await page.getByRole("button", { name: "Close navigation" }).click();
  }

  const ping = `screenshot ping ${testInfo.project.name}`;
  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill(ping);
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByText(`Echo: ${ping}`, { exact: true })).toBeVisible();
  await capture(page, testInfo, "04-message-flow");

  await page.getByRole("button", { name: "Members" }).click();
  await expect(page.getByLabel("Channel members")).toBeVisible();
  await capture(page, testInfo, "05-members-panel");
  await page.getByLabel("Channel members").getByRole("button", { name: "Close members" }).click();

  await page.getByRole("button", { name: "Agent settings" }).click();
  const agentPanel = page.getByLabel("Agent details");
  await expect(agentPanel).toBeVisible();
  await capture(page, testInfo, "06-agent-panel");

  await agentPanel.getByRole("tab", { name: /Files/ }).click();
  await agentPanel.getByLabel("File path").fill("screenshot-memory.md");
  await setMonacoEditorValue(page, agentPanel, "screenshot workspace note");
  await agentPanel.getByRole("button", { name: "Save file" }).click();
  await expect(agentPanel.getByText("Saved")).toBeVisible();
  await capture(page, testInfo, "10-agent-files-workspace");

  if (testInfo.project.name.startsWith("mobile")) {
    await agentPanel.getByRole("button", { name: "File tree" }).click();
    await expect(page.getByRole("dialog", { name: "Workspace file tree" })).toBeVisible();
    await capture(page, testInfo, "11-mobile-agent-file-tree-drawer");
  }
});

async function capture(page: Page, testInfo: TestInfo, name: string) {
  const projectName = testInfo.project.name.replace(/[^a-z0-9_-]+/gi, "-").toLowerCase();
  const dir = path.resolve(currentDir, "../../.agentx-screenshot", projectName);
  mkdirSync(dir, { recursive: true });
  const filePath = path.join(dir, `${name}.png`);
  await page.screenshot({ path: filePath, animations: "disabled" });
  await testInfo.attach(name, { path: filePath, contentType: "image/png" });
}

async function clearDefaultChannelMessages(page: Page) {
  await page.evaluate(async () => {
    const token = localStorage.getItem("agentx.session_token");
    if (!token) return;
    const headers = { Authorization: `Bearer ${token}` };
    const organizations = await fetch("/api/organizations", { headers }).then((r) => r.json());
    const organizationID = organizations[0]?.id;
    if (!organizationID) return;
    const projects = await fetch(`/api/organizations/${encodeURIComponent(organizationID)}/projects`, { headers }).then((r) => r.json());
    const projectID = projects[0]?.id;
    if (!projectID) return;
    const channels = await fetch(`/api/projects/${encodeURIComponent(projectID)}/channels`, { headers }).then((r) => r.json());
    const channelID = channels.find((channel: { name?: string }) => channel.name === "general")?.id ?? channels[0]?.id;
    if (!channelID) return;
    const messages = await fetch(`/api/conversations/channel/${encodeURIComponent(channelID)}/messages`, { headers }).then((r) => r.json());
    if (!Array.isArray(messages)) return;
    await Promise.all(
      messages.map((message: { id: string }) =>
        fetch(`/api/messages/${encodeURIComponent(message.id)}`, {
          method: "DELETE",
          headers,
        })
      )
    );
  });
}
