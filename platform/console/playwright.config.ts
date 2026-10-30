import { defineConfig } from '@playwright/test';

// Browser-level acceptance tests for the real compose stack. The CI lane
// installs Chromium; local macOS runs can use the installed Chrome with
// PLAYWRIGHT_CHANNEL=chrome. No webServer here: the deployment is the test.
export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 20_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.KUBONA_CONSOLE_URL || 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    ...(process.env.PLAYWRIGHT_CHANNEL
      ? { channel: process.env.PLAYWRIGHT_CHANNEL as 'chrome' | 'chromium' }
      : {}),
  },
});
