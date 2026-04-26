import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { seedDenseNavigation, setLightTheme } from "./helpers";

const adminToken = "e2e-token";
const currentDir = path.dirname(fileURLToPath(import.meta.url));

test.skip(!process.env.AGENTX_CAPTURE_SCREENSHOTS, "Optional diagnostic screenshots are disabled by default.");

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("captures diagnostic screenshots for AI review", async ({ page }, testInfo) => {
  await page.goto("/");
  await capture(page, testInfo, "01-login");

  await page.getByLabel("Admin token").fill(adminToken);
  await page.getByLabel("Display name").fill("Screenshot User");
  await page.getByRole("button", { name: "Enter" }).click();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await clearDefaultChannelMessages(page);
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await capture(page, testInfo, "02-shell-ready");

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
  await expect(page.getByLabel("Agent details")).toBeVisible();
  await capture(page, testInfo, "06-agent-panel");
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
