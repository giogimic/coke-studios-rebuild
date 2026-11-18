import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './browser',
  timeout: 30000,
  expect: {
    timeout: 5000,
  },
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:24902',
    trace: 'on-first-retry',
    viewport: { width: 1024, height: 768 },
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: process.platform === 'win32'
      ? '..\\tools\\bin\\coke-server.exe -addr 127.0.0.1:24902 -root ..\\sources\\gocoke\\web -db test_playwright.db'
      : '../tools/bin/coke-server-linux -addr 127.0.0.1:24902 -root ../sources/gocoke/web -db test_playwright.db',
    port: 24902,
    reuseExistingServer: true,
    timeout: 15000,
  },
});
