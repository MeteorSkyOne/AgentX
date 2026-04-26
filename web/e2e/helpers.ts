import type { Page, TestInfo } from "@playwright/test";

interface OrganizationSeed {
  id: string;
}

interface ProjectSeed {
  id: string;
}

interface ChannelSeed {
  id: string;
  name: string;
}

export interface DenseNavigationSeed {
  projectNames: string[];
  channelNames: string[];
}

export async function setLightTheme(page: Page) {
  await page.evaluate(() => {
    localStorage.setItem("agentx.theme", "light");
    document.documentElement.classList.remove("dark");
    document.documentElement.style.colorScheme = "light";
  });
}

export async function seedDenseNavigation(page: Page, testInfo: TestInfo): Promise<DenseNavigationSeed> {
  const stamp = `${slug(testInfo.project.name)}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

  return page.evaluate(async ({ stamp }) => {
    async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
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
    }

    const organizations = await request<OrganizationSeed[]>("/api/organizations");
    const organizationID = organizations[0]?.id;
    if (!organizationID) throw new Error("missing organization");

    const projects = await request<ProjectSeed[]>(`/api/organizations/${encodeURIComponent(organizationID)}/projects`);
    const projectID = projects[0]?.id;
    if (!projectID) throw new Error("missing project");

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
      await request<ProjectSeed>(`/api/organizations/${encodeURIComponent(organizationID)}/projects`, {
        method: "POST",
        body: JSON.stringify({ name }),
      });
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
      await request<ChannelSeed>(`/api/projects/${encodeURIComponent(projectID)}/channels`, {
        method: "POST",
        body: JSON.stringify({ name, type: "text" }),
      });
    }

    return { projectNames, channelNames };
  }, { stamp });
}

function slug(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
}
