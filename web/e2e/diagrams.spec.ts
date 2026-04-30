import { execFileSync } from "node:child_process";
import { expect, test, type Locator, type Page, type TestInfo } from "@playwright/test";
import {
  createChannelViaAPI,
  firstProject,
  request,
  signIn,
  writeWorkspaceFile,
} from "./helpers";

const mermaidSource = [
  "flowchart LR",
  "  MermaidAlpha[Mermaid Alpha] --> MermaidBeta[Mermaid Beta]",
].join("\n");

test.beforeAll(() => {
  try {
    execFileSync("d2", ["--version"], { stdio: "pipe" });
  } catch (err) {
    throw new Error(
      `D2 CLI is required for diagram e2e tests. Install d2 and ensure "d2 --version" succeeds. ${err instanceof Error ? err.message : ""}`
    );
  }
});

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("renders Mermaid and D2 diagrams in markdown messages", async ({ page }, testInfo) => {
  const d2Source = d2SourceForTest(testInfo);
  await signIn(page);
  const channel = await createChannelViaAPI(page, testInfo, {
    prefix: "diagrams",
    bindDefaultAgent: false,
  });
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
  await selectChannel(page, channel.name);

  await sendMarkdownMessage(page, "mermaid diagram e2e", "mermaid", mermaidSource);
  const mermaidMessage = page
    .getByLabel("Messages")
    .getByTestId("message-body")
    .filter({ hasText: "mermaid diagram e2e" })
    .first();
  const mermaidDiagram = mermaidMessage.getByTestId("mermaid-diagram").first();
  await expectMermaidDiagram(mermaidDiagram);
  await attachLocatorScreenshot(testInfo, "mermaid-message-diagram", mermaidDiagram);

  const firstD2Response = page.waitForResponse((response) =>
    response.url().includes("/api/diagrams/d2/render") && response.request().method() === "POST"
  );
  await sendMarkdownMessage(page, "d2 diagram e2e first", "d2", d2Source);
  const firstD2Payload = (await (await firstD2Response).json()) as { cached: boolean };
  expect(firstD2Payload.cached).toBe(false);

  const firstD2Message = page
    .getByLabel("Messages")
    .getByTestId("message-body")
    .filter({ hasText: "d2 diagram e2e first" })
    .first();
  const firstD2Diagram = firstD2Message.getByTestId("d2-diagram").first();
  await expectD2Diagram(firstD2Diagram);
  await attachLocatorScreenshot(testInfo, "d2-message-diagram", firstD2Diagram);
  await expectD2PreviewZoom(page, firstD2Diagram);

  const cachedD2Payload = await request<{ cached: boolean; svg: string }>(
    page,
    "/api/diagrams/d2/render",
    {
      method: "POST",
      body: JSON.stringify({ source: d2Source }),
    }
  );
  expect(cachedD2Payload.cached).toBe(true);
  expect(cachedD2Payload.svg).toContain("<svg");

  await sendMarkdownMessage(page, "d2 diagram e2e second", "d2", d2Source);
  const secondD2Message = page
    .getByLabel("Messages")
    .getByTestId("message-body")
    .filter({ hasText: "d2 diagram e2e second" })
    .first();
  await expectD2Diagram(secondD2Message.getByTestId("d2-diagram").first());
});

test("renders Mermaid and D2 diagrams in markdown file preview", async ({ page }, testInfo) => {
  const d2Source = d2SourceForTest(testInfo);
  await signIn(page);
  const project = await firstProject(page);
  await writeWorkspaceFile(
    page,
    project.workspace_id,
    "diagrams.md",
    [
      "# Diagram preview",
      "",
      "```mermaid",
      mermaidSource,
      "```",
      "",
      "```d2",
      d2Source,
      "```",
    ].join("\n")
  );

  await page.getByRole("button", { name: "Project files" }).click();
  const projectTree = page.getByRole("tree", { name: "Project files" });
  await expect(projectTree).toBeVisible();
  await projectTree.getByRole("treeitem", { name: "diagrams.md" }).click();
  await expect(page.getByTestId("project-file-editor-pane")).toBeVisible();
  await page.getByRole("button", { name: "Preview Markdown" }).click();

  const preview = page.getByTestId("workspace-file-markdown-preview");
  await expect(preview).toBeVisible();
  const mermaidDiagram = preview.getByTestId("mermaid-diagram").first();
  const d2Diagram = preview.getByTestId("d2-diagram").first();
  await expectMermaidDiagram(mermaidDiagram);
  await expectD2Diagram(d2Diagram);
  await attachLocatorScreenshot(testInfo, "mermaid-preview-diagram", mermaidDiagram);
  await attachLocatorScreenshot(testInfo, "d2-preview-diagram", d2Diagram);
});

async function sendMarkdownMessage(
  page: Page,
  marker: string,
  language: "mermaid" | "d2",
  source: string
) {
  await page.getByRole("textbox", { name: "Message" }).fill([
    marker,
    "",
    "```" + language,
    source,
    "```",
  ].join("\n"));
  await page.getByRole("button", { name: "Send" }).click();
}

function d2SourceForTest(testInfo: TestInfo): string {
  const suffix = `${testInfo.project.name} ${testInfo.title} ${testInfo.retry}`
    .replace(/[^A-Za-z0-9_]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 96);
  const alpha = `D2Alpha_${suffix}`;
  const beta = `D2Beta_${suffix}`;
  return [
    "direction: down",
    `${alpha}: client`,
    `${beta}: server`,
    `${alpha} -> ${beta}: HTTP Request`,
    `${beta} -> ${alpha}: HTTP Response`,
  ].join("\n");
}

async function selectChannel(page: Page, channelName: string) {
  const channelButton = page.getByRole("button", { name: channelName, exact: true });
  if (await channelButton.isVisible().catch(() => false)) {
    await channelButton.click();
    return;
  }

  await page.getByRole("button", { name: "Navigation" }).click();
  const navigation = page.getByRole("dialog", { name: "Navigation" });
  await expect(navigation).toBeVisible();
  await navigation.getByRole("button", { name: channelName, exact: true }).click();
  await expect(navigation).toHaveCount(0);
}

async function expectMermaidDiagram(diagram: Locator) {
  await expect(diagram).toBeVisible({ timeout: 15_000 });
  const svg = diagram.locator("svg").first();
  await expect(svg).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(() =>
      svg.evaluate((node) => {
        const text = node.textContent ?? "";
        return text.includes("Mermaid Alpha") ||
          text.includes("Mermaid") ||
          node.querySelectorAll("text").length > 0;
      })
    )
    .toBe(true);
  await expectVisibleSize(svg);
}

async function expectD2Diagram(diagram: Locator) {
  await expect(diagram).toBeVisible({ timeout: 15_000 });
  const image = diagram.getByTestId("d2-diagram-image");
  await expect(image).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(() =>
      image.evaluate((node) => {
        const img = node as HTMLImageElement;
        const rect = img.getBoundingClientRect();
        return img.complete &&
          img.naturalWidth > 0 &&
          img.naturalHeight > 0 &&
          rect.width > 0 &&
          rect.height > 0;
      })
    )
    .toBe(true);
  const metrics = await image.evaluate((node) => {
    const img = node as HTMLImageElement;
    const rect = img.getBoundingClientRect();
    return {
      naturalWidth: img.naturalWidth,
      naturalHeight: img.naturalHeight,
      width: rect.width,
      height: rect.height,
    };
  });
  expect(metrics.naturalWidth).toBeGreaterThan(0);
  expect(metrics.naturalHeight).toBeGreaterThan(0);
  expect(metrics.width).toBeGreaterThan(0);
  expect(metrics.height).toBeGreaterThan(0);

  const containerMetrics = await diagram.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    return {
      height: rect.height,
      viewportHeight: window.innerHeight,
    };
  });
  expect(containerMetrics.height).toBeLessThanOrEqual(
    Math.ceil(containerMetrics.viewportHeight * 0.65)
  );
}

async function expectD2PreviewZoom(page: Page, diagram: Locator) {
  await diagram.getByRole("button", { name: "Open D2 diagram preview" }).first().click();
  const dialog = page.getByRole("dialog", { name: "D2 Diagram" });
  await expect(dialog).toBeVisible({ timeout: 15_000 });

  const viewport = dialog.getByTestId("d2-diagram-preview-viewport");
  const canvas = dialog.getByTestId("d2-diagram-preview-canvas");
  const image = dialog.getByTestId("d2-diagram-preview-image");
  await expect(image).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(() =>
      image.evaluate((node) => {
        const img = node as HTMLImageElement;
        return img.complete && img.naturalWidth > 0 && img.style.width !== "";
      })
    )
    .toBe(true);

  const viewportOverflow = await viewport.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      overflowX: style.overflowX,
      overflowY: style.overflowY,
    };
  });
  expect(viewportOverflow.overflowX).toBe("hidden");
  expect(viewportOverflow.overflowY).toBe("hidden");
  await expect(canvas).toHaveCSS("position", "relative");

  const widthBefore = await previewImageWidth(image);
  await dialog.getByRole("button", { name: "Zoom in D2 diagram" }).click();
  await expect.poll(() => previewImageWidth(image)).toBeGreaterThan(widthBefore);

  const widthAfterButton = await previewImageWidth(image);
  await viewport.dispatchEvent("wheel", { deltaY: -260 });
  await expect.poll(() => previewImageWidth(image)).toBeGreaterThan(widthAfterButton);

  const viewportBox = await viewport.boundingBox();
  if (!viewportBox) {
    throw new Error("D2 preview bounds are unavailable");
  }
  await viewport.dispatchEvent("wheel", { deltaY: -1200 });
  await expect.poll(() => previewImageWidth(image)).toBeGreaterThan(viewportBox.width + 80);

  const imageBoxBeforeDrag = await image.boundingBox();
  if (!imageBoxBeforeDrag) {
    throw new Error("D2 preview image bounds are unavailable");
  }
  await page.mouse.move(
    viewportBox.x + viewportBox.width / 2,
    viewportBox.y + viewportBox.height / 2
  );
  await page.mouse.down();
  await page.mouse.move(
    viewportBox.x + viewportBox.width / 2 + 80,
    viewportBox.y + viewportBox.height / 2 + 40
  );
  await page.mouse.up();
  await expect
    .poll(async () => (await image.boundingBox())?.x ?? Number.NEGATIVE_INFINITY)
    .toBeGreaterThan(imageBoxBeforeDrag.x + 20);

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
}

async function previewImageWidth(image: Locator): Promise<number> {
  return image.evaluate((node) => {
    const img = node as HTMLImageElement;
    const width = Number.parseFloat(img.style.width);
    return Number.isFinite(width) && width > 0 ? width : img.getBoundingClientRect().width;
  });
}

async function expectVisibleSize(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.width ?? 0).toBeGreaterThan(0);
  expect(box?.height ?? 0).toBeGreaterThan(0);
}

async function attachLocatorScreenshot(testInfo: TestInfo, name: string, locator: Locator) {
  await testInfo.attach(name, {
    body: await locator.screenshot({ animations: "disabled" }),
    contentType: "image/png",
  });
}
