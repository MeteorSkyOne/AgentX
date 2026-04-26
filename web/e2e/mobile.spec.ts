import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { seedDenseNavigation, setLightTheme } from "./helpers";

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

  const channelName = `${type}-${slug(testInfo.project.name)}-${Date.now()}`;
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
  await composer.fill("mobile ping");
  await page.getByRole("button", { name: "Send" }).click();
  const messages = page.getByLabel("Messages");
  await expect(messages.getByText("mobile ping", { exact: true })).toBeVisible();
  await expect(messages.getByText("Echo: mobile ping", { exact: true })).toBeVisible();
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
  await page.getByLabel("Post title").fill(`Mobile post ${slug(testInfo.project.name)}`);
  await page.getByLabel("Post body").fill("thread hello from mobile");
  await page.getByRole("button", { name: "Create post" }).click();

  await expect(page.getByRole("heading", { name: /Mobile post/ })).toBeVisible();
  await expect(page.getByText("thread hello from mobile", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Back to posts" })).toBeVisible();
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

  const headerControl = nav.getByRole("button", { name: "Close navigation" });
  const footer = nav.getByTestId("mobile-nav-footer");
  await expect(headerControl).toBeInViewport({ ratio: 1 });
  await expect(footer).toBeInViewport({ ratio: 1 });
  await expect(footer.getByRole("button", { name: "User settings" })).toBeInViewport({ ratio: 1 });
  await expect(footer.getByRole("button", { name: "Switch to dark mode" })).toBeInViewport({ ratio: 1 });
  await expect(footer.getByRole("button", { name: "Log out" })).toBeInViewport({ ratio: 1 });

  const headerBefore = await headerControl.boundingBox();
  const footerBefore = await footer.boundingBox();
  await scrollViewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  const headerAfter = await headerControl.boundingBox();
  const footerAfter = await footer.boundingBox();
  expect(headerBefore).not.toBeNull();
  expect(headerAfter).not.toBeNull();
  expect(footerBefore).not.toBeNull();
  expect(footerAfter).not.toBeNull();
  expect(Math.abs(headerAfter!.y - headerBefore!.y)).toBeLessThanOrEqual(2);
  expect(Math.abs(footerAfter!.y - footerBefore!.y)).toBeLessThanOrEqual(2);
  await expect(footer).toBeInViewport({ ratio: 1 });
  await expectNoHorizontalOverflow(page);
});

function slug(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}
