import { defineConfig, devices } from "@playwright/test";

const localNoProxyEntries = ["127.0.0.1", "localhost", "::1"];

function mergeNoProxy(...values: Array<string | undefined>): string {
  const entries = new Set<string>();
  for (const value of values) {
    for (const entry of (value ?? "").split(",")) {
      const trimmed = entry.trim();
      if (trimmed) entries.add(trimmed);
    }
  }
  for (const entry of localNoProxyEntries) {
    entries.add(entry);
  }
  return Array.from(entries).join(",");
}

const noProxy = mergeNoProxy(process.env.NO_PROXY, process.env.no_proxy);
process.env.NO_PROXY = noProxy;
process.env.no_proxy = noProxy;

const e2eHost = "127.0.0.1";
const e2eApiPort = 19080;
const e2eWebPort = 15174;
const e2eApiOrigin = `http://${e2eHost}:${e2eApiPort}`;
const e2eWebOrigin = `http://${e2eHost}:${e2eWebPort}`;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  expect: {
    timeout: 10_000
  },
  use: {
    baseURL: e2eWebOrigin,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure"
  },
  reporter: [["list"], ["html", { open: "never" }]],
  webServer: [
    {
      name: "agentx-api",
      command:
        `cd .. && rm -rf .agentx-e2e && mkdir -p .agentx-e2e && AGENTX_ADDR=${e2eHost}:${e2eApiPort} AGENTX_DATA_DIR=.agentx-e2e AGENTX_SQLITE_PATH=.agentx-e2e/agentx.db AGENTX_ADMIN_TOKEN=e2e-token go run ./cmd/agentx`,
      url: `${e2eApiOrigin}/healthz`,
      timeout: 30_000,
      reuseExistingServer: false
    },
    {
      name: "agentx-web",
      command:
        `AGENTX_API_TARGET=${e2eApiOrigin} pnpm exec vite --host ${e2eHost} --port ${e2eWebPort} --strictPort`,
      url: e2eWebOrigin,
      timeout: 30_000,
      reuseExistingServer: false
    }
  ],
  projects: [
    {
      name: "desktop-chromium",
      testMatch: /(?:app|screenshots)\.spec\.ts/,
      use: { ...devices["Desktop Chrome"] }
    },
    {
      name: "mobile-chrome",
      testMatch: /(?:mobile|screenshots)\.spec\.ts/,
      use: { ...devices["Pixel 5"], browserName: "chromium" }
    },
    {
      name: "mobile-iphone",
      testMatch: /(?:mobile|screenshots)\.spec\.ts/,
      use: { ...devices["iPhone 12"], browserName: "chromium" }
    }
  ]
});
