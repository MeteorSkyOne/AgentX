import { expect, test, type Page } from "@playwright/test";

const adminToken = "e2e-token";
const displayName = "E2E User";

async function signIn(page: Page, name = displayName) {
  await page.goto("/");

  await page.getByLabel("Admin token").fill(adminToken);
  await page.getByLabel("Display name").fill(name);
  await page.getByRole("button", { name: "Enter" }).click();

  await expect(page.getByRole("button", { name: /general/i })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
}

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("validates login, restores the session, and logs back in", async ({ page }) => {
  await page.evaluate(() => localStorage.setItem("agentx.session_token", "invalid-token"));
  await page.goto("/");
  await expect(page.getByLabel("Admin token")).toBeVisible();

  await expect(page.getByRole("button", { name: "Enter" })).toBeDisabled();
  await page.getByLabel("Admin token").fill("wrong-token");
  await page.getByRole("button", { name: "Enter" }).click();
  await expect(page.getByText("unauthorized")).toBeVisible();

  await page.getByLabel("Admin token").fill(adminToken);
  await page.getByLabel("Display name").fill(displayName);
  await page.getByRole("button", { name: "Enter" }).click();

  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Default" })).toBeVisible();
  await expect(page.getByText(displayName)).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();

  await page.getByRole("button", { name: "Log out" }).click();
  await expect(page.getByLabel("Admin token")).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message" })).toHaveCount(0);
  expect(await page.evaluate(() => localStorage.getItem("agentx.session_token"))).toBeNull();

  await page.getByLabel("Admin token").fill(adminToken);
  await page.getByRole("button", { name: "Enter" }).click();
  await expect(page.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
});

test("sends messages from the composer and receives agent output", async ({ page }) => {
  await signIn(page);

  await expect(page.getByText("No messages yet")).toBeVisible();

  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill("button ping");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("button ping", { exact: true })).toBeVisible();
  await expect(page.getByText("Echo: button ping", { exact: true })).toBeVisible();
  await expect(composer).toHaveValue("");

  await composer.fill("enter ping");
  await composer.press("Enter");

  await expect(page.getByText("enter ping", { exact: true })).toBeVisible();
  await expect(page.getByText("Echo: enter ping", { exact: true })).toBeVisible();

  await composer.fill("line one");
  await composer.press("Shift+Enter");
  await expect(composer).toHaveValue("line one\n");
  await page.keyboard.insertText("line two");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("line one")).toBeVisible();
  await expect(page.getByText("line two")).toBeVisible();
});

test("opens and closes the bound agent details panel", async ({ page }) => {
  await signIn(page);

  await page.getByLabel("Bound agent").getByRole("button", { name: /Fake Agent/ }).click();
  await expect(page.getByLabel("Agent details")).toBeVisible();
  await page.getByLabel("Agent details").getByRole("button", { name: "Close" }).click();
  await expect(page.getByLabel("Agent details")).toHaveCount(0);

  await page.getByRole("button", { name: "Agent settings" }).click();
  const agentPanel = page.getByLabel("Agent details");
  await expect(agentPanel.getByRole("heading", { name: "Fake Agent" })).toBeVisible();
  await expect(agentPanel.getByText("Fake runtime")).toBeVisible();
  await expect(agentPanel.getByText("fake-echo")).toBeVisible();
  await expect(agentPanel.getByText("channel")).toBeVisible();
  await expect(agentPanel.locator(".workspace-path").getByText(/fake-default/)).toBeVisible();
  await expect(agentPanel.getByText("empty")).toBeVisible();

  await page.getByRole("button", { name: "Agent settings" }).click();
  await expect(page.getByLabel("Agent details")).toHaveCount(0);
});

test("manages projects, channel agents, threads, and workspace files", async ({ page }) => {
  await signIn(page);

  await page.getByRole("button", { name: "Create project" }).click();
  const projectModal = page.getByRole("dialog");
  await projectModal.getByLabel("Project name").fill("Ops");
  await projectModal.getByRole("button", { name: "Save" }).click();
  await expect(page.getByRole("button", { name: "Ops" })).toBeVisible();

  await page.getByRole("button", { name: "Create channel" }).click();
  const channelModal = page.getByRole("dialog");
  await channelModal.getByLabel("Channel name").fill("lab");
  await channelModal.getByLabel("Channel type").selectOption("text");
  await channelModal.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByRole("button", { name: "lab" })).toBeVisible();

  // Create a new agent via modal
  await page.getByRole("button", { name: "Create agent" }).click();
  const agentModal = page.getByRole("dialog");
  await agentModal.getByLabel("New agent name").fill("Agent Two");
  await agentModal.getByLabel("New agent handle").fill("agent_two");
  await agentModal.getByRole("button", { name: "Create", exact: true }).click();

  // Open agent panel and verify the new agent exists
  await page.getByRole("button", { name: "Agent settings" }).click();
  const panel = page.getByLabel("Agent details");
  await expect(panel.getByLabel("Agent", { exact: true })).toContainText("Agent Two");

  // Switch to Members tab and manage channel bindings
  await panel.getByRole("tab", { name: /Members/ }).click();
  await panel.locator(".picker-row").filter({ hasText: "Fake Agent" }).getByRole("checkbox").check();
  await panel.locator(".picker-row").filter({ hasText: "Agent Two" }).getByRole("checkbox").check();
  await panel.getByRole("button", { name: "Save" }).click();
  await expect(page.getByLabel("Bound agent").getByRole("button", { name: "Agent Two" })).toBeVisible();

  // Close panel, test multi-agent messaging
  await page.getByRole("button", { name: "Agent settings" }).click();
  const composer = page.getByRole("textbox", { name: "Message" });
  await composer.fill("multi ping");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByText("Echo: multi ping", { exact: true })).toHaveCount(2);

  await composer.fill("@ag");
  await page.getByRole("button", { name: /Agent Two.*@agent_two/ }).click();
  await expect(composer).toHaveValue("@agent_two ");
  await page.keyboard.insertText("directed");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByText("Echo: @agent_two directed", { exact: true })).toHaveCount(1);
  await expect(page.locator('[data-mention="agent_two"]').first()).toBeVisible();

  // Open panel, switch to Files tab for workspace file management
  await page.getByRole("button", { name: "Agent settings" }).click();
  await panel.getByLabel("Agent", { exact: true }).selectOption({ label: "Agent Two (@agent_two)" });
  await panel.getByRole("tab", { name: /Files/ }).click();
  await panel.getByLabel("File path").fill("memory.md");
  await panel.getByLabel("File content").fill("memory from e2e");
  await panel.getByRole("button", { name: "Save file" }).click();
  await panel.getByLabel("File content").fill("");
  await panel.getByRole("button", { name: "Open" }).click();
  await expect(panel.getByLabel("File content")).toHaveValue("memory from e2e");
  await page.getByRole("button", { name: "Agent settings" }).click();

  // Create a thread/forum channel
  await page.getByRole("button", { name: "Create channel" }).click();
  await page.getByLabel("Channel name").fill("posts");
  await page.getByLabel("Channel type").selectOption("thread");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByLabel("Threads")).toBeVisible();

  await page.getByLabel("Post title").fill("First post");
  await page.getByLabel("Post body").fill("thread hello");
  await page.getByRole("button", { name: "Create post" }).click();
  await expect(page.getByRole("heading", { name: "First post" })).toBeVisible();
  await expect(page.getByText("thread hello", { exact: true })).toBeVisible();
});

test("confirms before deleting an agent", async ({ page }) => {
  await signIn(page);

  await page.getByRole("button", { name: "Create agent" }).click();
  const agentModal = page.getByRole("dialog");
  await agentModal.getByLabel("New agent name").fill("Disposable");
  await agentModal.getByLabel("New agent handle").fill("disposable");
  await agentModal.getByRole("button", { name: "Create", exact: true }).click();

  await page.getByRole("button", { name: "Agent settings" }).click();
  const panel = page.getByLabel("Agent details");
  await panel.getByLabel("Agent", { exact: true }).selectOption({ label: "Disposable (@disposable)" });
  await panel.getByRole("button", { name: "Delete agent" }).click();

  const confirm = page.getByRole("dialog", { name: "Delete agent?" });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Cancel" }).click();
  await expect(confirm).toHaveCount(0);
  await expect(panel.getByLabel("Agent", { exact: true })).toContainText("Disposable");

  await panel.getByRole("button", { name: "Delete agent" }).click();
  await page.getByRole("dialog", { name: "Delete agent?" }).getByRole("button", { name: "Delete" }).click();
  await expect(panel.getByText("Deleted")).toBeVisible();

  await page.getByRole("button", { name: "Agent settings" }).click();
  await page.getByRole("button", { name: "Members" }).click();
  await expect(page.getByLabel("Channel members").getByText("Disposable")).toHaveCount(0);

  await page.getByRole("button", { name: "Create agent" }).click();
  await page.getByRole("dialog").getByLabel("New agent name").fill("Disposable");
  await page.getByRole("dialog").getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByText("Agent @disposable already exists. Choose a different handle.")).toBeVisible();
});
