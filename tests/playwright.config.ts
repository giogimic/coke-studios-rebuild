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
});
