import { expect, test } from "@playwright/test";
import { setLightTheme, signIn } from "./helpers";

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => localStorage.clear());
});

test("renders markdown code blocks with a light panel in light mode", async ({ page }) => {
  await signIn(page);
  await setLightTheme(page);
  await page.reload();
  await expect(page.locator("html")).not.toHaveClass(/dark/);
  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();

  const marker = "markdown code block light screenshot";
  await page.getByRole("textbox", { name: "Message" }).fill([
    marker,
    "",
    "```",
    "| App A |  App B |",
    "|       |        |",
    "```",
  ].join("\n"));
  await page.getByRole("button", { name: "Send" }).click();

  const message = page
    .getByLabel("Messages")
    .getByTestId("message-body")
    .filter({ hasText: marker })
    .first();
  const codeBlock = message.getByTestId("code-block-shell").first();
  await expect(codeBlock).toBeVisible();
  await expect(codeBlock).not.toHaveCSS("background-color", "rgb(40, 44, 52)");

  await expect(codeBlock).toHaveScreenshot("markdown-code-block-light.png", {
    animations: "disabled",
    maxDiffPixels: 20,
  });
});
