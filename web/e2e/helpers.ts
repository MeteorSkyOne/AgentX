import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";

interface OrganizationSeed {
  id: string;
}

export interface ProjectSeed {
  id: string;
  name: string;
  workspace_id: string;
}

interface ChannelSeed {
  id: string;
  name: string;
  type: "text" | "thread";
}

export interface AgentSeed {
  id: string;
  name: string;
  handle: string;
  kind: string;
  enabled: boolean;
  config_workspace_id: string;
  default_workspace_id: string;
}

interface E2ERequestInit {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
}

export interface DenseNavigationSeed {
  projectNames: string[];
  channelNames: string[];
}

export const e2eSetupToken = "e2e-token";
export const e2eUsername = "e2e_admin";
export const e2ePassword = "e2e-password-1234";

export async function signIn(page: Page, displayName = "E2E User") {
  await page.goto("/");
  await expect(page.getByLabel("Username")).toBeVisible();

  if (await page.getByLabel("Setup token").isVisible().catch(() => false)) {
    await page.getByLabel("Setup token").fill(e2eSetupToken);
    await page.getByLabel("Username").fill(e2eUsername);
    await page.getByLabel("Display name").fill(displayName);
    await page.getByLabel("Password", { exact: true }).fill(e2ePassword);
    await page.getByLabel("Confirm password").fill(e2ePassword);
    await page.getByRole("button", { name: "Set up admin" }).click();
  } else {
    await page.getByLabel("Username").fill(e2eUsername);
    await page.getByLabel("Password", { exact: true }).fill(e2ePassword);
    await page.getByRole("button", { name: "Log in" }).click();
  }

  await expect(page.getByRole("textbox", { name: "Message" })).toBeEnabled();
}

export async function setLightTheme(page: Page) {
  await page.evaluate(() => {
    localStorage.setItem("agentx.theme", "light");
    document.documentElement.classList.remove("dark");
    document.documentElement.style.colorScheme = "light";
  });
}

export async function setMonacoEditorValue(page: Page, scope: Locator, value: string) {
  const editor = scope.getByTestId("workspace-file-editor");
  const monacoEditor = editor.locator(".monaco-editor");
  await expect(monacoEditor).toBeVisible({ timeout: 15_000 });
  const editorHandle = await editor.elementHandle();
  const hasEditorHook = editorHandle
    ? await page
        .waitForFunction(
          (node) => {
            const editorNode = node as HTMLElement & {
              __agentxSetEditorValue?: (value: string) => void;
            };
            return Boolean(editorNode.__agentxSetEditorValue);
          },
          editorHandle,
          { timeout: 5_000 }
        )
        .then(() => true)
        .catch(() => false)
    : false;

  if (hasEditorHook) {
    await editor.evaluate((node, nextValue) => {
      const editorNode = node as HTMLElement & {
        __agentxSetEditorValue?: (value: string) => void;
      };
      editorNode.__agentxSetEditorValue?.(nextValue);
    }, value);
    await expect
      .poll(() =>
        editor.evaluate((node) => {
          const editorNode = node as HTMLElement & {
            __agentxGetEditorValue?: () => string;
          };
          return editorNode.__agentxGetEditorValue?.() ?? "";
        })
      )
      .toBe(value);
    return;
  }

  await monacoEditor.click();
  await page.keyboard.press(`${keyboardModifier()}+A`);
  await page.keyboard.press("Backspace");
  if (value) {
    await page.keyboard.insertText(value);
  }
}

export async function expectMonacoEditorText(scope: Locator, text: string) {
  const editor = scope.getByTestId("workspace-file-editor");
  await expect(editor.locator(".view-line").filter({ hasText: text }).first()).toBeVisible({
    timeout: 15_000,
  });
}

export async function expectMonacoEditorPosition(
  page: Page,
  scope: Locator,
  position: { lineNumber: number; column: number }
) {
  const editor = scope.getByTestId("workspace-file-editor");
  await expect(editor.locator(".monaco-editor")).toBeVisible({ timeout: 15_000 });
  const editorHandle = await editor.elementHandle();
  if (!editorHandle) throw new Error("missing workspace editor handle");

  await page.waitForFunction(
    (node) => {
      const editorNode = node as HTMLElement & {
        __agentxGetEditorPosition?: () => { lineNumber: number; column: number } | null;
      };
      return Boolean(editorNode.__agentxGetEditorPosition);
    },
    editorHandle,
    { timeout: 5_000 }
  );

  await expect
    .poll(() =>
      editor.evaluate((node) => {
        const editorNode = node as HTMLElement & {
          __agentxGetEditorPosition?: () => { lineNumber: number; column: number } | null;
        };
        return editorNode.__agentxGetEditorPosition?.() ?? null;
      })
    )
    .toEqual(position);
}

export async function request<T>(
  page: Page,
  path: string,
  init: E2ERequestInit = {}
): Promise<T> {
  return page.evaluate(
    async ({ path, init }) => {
      const token = localStorage.getItem("agentx.session_token");
      if (!token) throw new Error("missing session token");

      const headers = new Headers(init.headers);
      headers.set("Authorization", `Bearer ${token}`);
      if (init.body && !headers.has("Content-Type")) {
        headers.set("Content-Type", "application/json");
      }

      const response = await fetch(path, { ...init, headers });
      if (!response.ok) {
        throw new Error(`${response.status} ${await response.text()}`);
      }
      if (response.status === 204) {
        return undefined as T;
      }
      return response.json() as Promise<T>;
    },
    { path, init }
  );
}

export function uniqueName(testInfo: TestInfo, prefix: string): string {
  return `${prefix} ${uniqueSuffix(testInfo)}`;
}

export function uniqueHandle(testInfo: TestInfo, prefix: string): string {
  return `${slug(prefix)}_${uniqueSuffix(testInfo, "_")}`;
}

export async function createProjectViaAPI(
  page: Page,
  testInfo: TestInfo,
  prefix = "Project"
): Promise<ProjectSeed> {
  const organizationID = await firstOrganizationID(page);
  return request<ProjectSeed>(
    page,
    `/api/organizations/${encodeURIComponent(organizationID)}/projects`,
    {
      method: "POST",
      body: JSON.stringify({ name: uniqueName(testInfo, prefix) }),
    }
  );
}

export async function createChannelViaAPI(
  page: Page,
  testInfo: TestInfo,
  options: {
    projectID?: string;
    prefix?: string;
    type?: "text" | "thread";
    bindDefaultAgent?: boolean;
  } = {}
): Promise<ChannelSeed> {
  const projectID = options.projectID ?? (await firstProject(page)).id;
  const type = options.type ?? "text";
  const channel = await request<ChannelSeed>(
    page,
    `/api/projects/${encodeURIComponent(projectID)}/channels`,
    {
      method: "POST",
      body: JSON.stringify({
        name: uniqueName(testInfo, options.prefix ?? type),
        type,
      }),
    }
  );
  if (options.bindDefaultAgent !== false) {
    const agent = await firstEnabledAgent(page);
    await request(
      page,
      `/api/channels/${encodeURIComponent(channel.id)}/agents`,
      {
        method: "PUT",
        body: JSON.stringify({ agents: [{ agent_id: agent.id }] }),
      }
    );
  }
  return channel;
}

export async function createAgentViaAPI(
  page: Page,
  testInfo: TestInfo,
  options: {
    name?: string;
    handle?: string;
    kind?: string;
    env?: Record<string, string>;
  } = {}
): Promise<AgentSeed> {
  const organizationID = await firstOrganizationID(page);
  const name = options.name ?? uniqueName(testInfo, "Agent");
  return request<AgentSeed>(
    page,
    `/api/organizations/${encodeURIComponent(organizationID)}/agents`,
    {
      method: "POST",
      body: JSON.stringify({
        name,
        handle: options.handle,
        kind: options.kind,
        env: options.env,
      }),
    }
  );
}

export async function seedDenseNavigation(page: Page, testInfo: TestInfo): Promise<DenseNavigationSeed> {
  const stamp = uniqueSuffix(testInfo, "_");
  const organizationID = await firstOrganizationID(page);
  const projectID = (await firstProject(page)).id;

  const projectNames = [
    `1 ${stamp}`,
    `Mobile ops ${stamp}`,
    `Research ${stamp}`,
    `Archive ${stamp}`,
    `Support ${stamp}`,
    `QA ${stamp}`,
    `Release ${stamp}`,
    `Design ${stamp}`,
  ];

  for (const name of projectNames) {
    await request<ProjectSeed>(
      page,
      `/api/organizations/${encodeURIComponent(organizationID)}/projects`,
      {
        method: "POST",
        body: JSON.stringify({ name }),
      }
    );
  }

  const channelNames = [
    `a ${stamp}`,
    `claude ${stamp}`,
    `codex ${stamp}`,
    `500 message load test ${stamp.replace(/_/g, "-")}`,
    `review ${stamp}`,
    `incident ${stamp}`,
    `handoff ${stamp}`,
    `triage ${stamp}`,
    `deploy ${stamp}`,
    `research ${stamp}`,
    `qa ${stamp}`,
    `design ${stamp}`,
    `support ${stamp}`,
    `ops ${stamp}`,
    `metrics ${stamp}`,
    `feedback ${stamp}`,
    `planning ${stamp}`,
    `archive ${stamp}`,
  ];

  for (const name of channelNames) {
    await request<ChannelSeed>(
      page,
      `/api/projects/${encodeURIComponent(projectID)}/channels`,
      {
        method: "POST",
        body: JSON.stringify({ name, type: "text" }),
      }
    );
  }

  return { projectNames, channelNames };
}

export function writeWorkspaceFile(
  page: Page,
  workspaceID: string,
  filePath: string,
  body: string
): Promise<unknown> {
  return request(
    page,
    `/api/workspaces/${encodeURIComponent(workspaceID)}/files?${new URLSearchParams({ path: filePath }).toString()}`,
    {
      method: "PUT",
      body: JSON.stringify({ body }),
    }
  );
}

export async function readWorkspaceFile(
  page: Page,
  workspaceID: string,
  filePath: string
): Promise<string> {
  const file = await request<{ body: string }>(
    page,
    `/api/workspaces/${encodeURIComponent(workspaceID)}/files?${new URLSearchParams({ path: filePath }).toString()}`
  );
  return file.body;
}

async function firstOrganizationID(page: Page): Promise<string> {
  const organizations = await request<OrganizationSeed[]>(page, "/api/organizations");
  const organizationID = organizations[0]?.id;
  if (!organizationID) throw new Error("missing organization");
  return organizationID;
}

export async function firstProject(page: Page): Promise<ProjectSeed> {
  const organizationID = await firstOrganizationID(page);
  const projects = await request<ProjectSeed[]>(
    page,
    `/api/organizations/${encodeURIComponent(organizationID)}/projects`
  );
  const project = projects[0];
  if (!project) throw new Error("missing project");
  return project;
}

export async function firstEnabledAgent(page: Page): Promise<AgentSeed> {
  const organizationID = await firstOrganizationID(page);
  const agents = await request<AgentSeed[]>(
    page,
    `/api/organizations/${encodeURIComponent(organizationID)}/agents`
  );
  const agent = agents.find((item) => item.enabled) ?? agents[0];
  if (!agent) throw new Error("missing agent");
  return agent;
}

function uniqueSuffix(testInfo: TestInfo, separator = "-"): string {
  return [
    slug(testInfo.project.name),
    Date.now().toString(36),
    Math.random().toString(36).slice(2, 8),
  ].join(separator);
}

function keyboardModifier(): "Control" | "Meta" {
  return process.platform === "darwin" ? "Meta" : "Control";
}

function slug(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
}
