import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  expect: {
    timeout: 10_000
  },
  use: {
    baseURL: "http://127.0.0.1:5174",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure"
  },
  reporter: [["list"], ["html", { open: "never" }]],
  webServer: [
    {
      command:
        "cd .. && rm -rf .agentx-e2e && mkdir -p .agentx-e2e && AGENTX_ADDR=127.0.0.1:18080 AGENTX_DATA_DIR=.agentx-e2e AGENTX_SQLITE_PATH=.agentx-e2e/agentx.db AGENTX_ADMIN_TOKEN=e2e-token go run ./cmd/agentx",
      url: "http://127.0.0.1:18080/healthz",
      timeout: 30_000,
      reuseExistingServer: false
    },
    {
      command:
        "AGENTX_API_TARGET=http://127.0.0.1:18080 pnpm run dev -- --host 127.0.0.1 --port 5174",
      url: "http://127.0.0.1:5174",
      timeout: 30_000,
      reuseExistingServer: false
    }
  ],
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] }
    }
  ]
});
