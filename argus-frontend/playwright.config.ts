// =============================================================================
// Argus AI — Playwright E2E Configuration
// =============================================================================
//
// Targets: Chromium only (IndexedDB + SPA focus).
// Strategy: Page-level E2E with mocked API routes.
//           No running Go backend required — all HTTP intercepted.
//
// Usage:
//   npx playwright test                          # run all
//   npx playwright test tests/resiliency/        # run resiliency suite
//   npx playwright test --ui                     # interactive UI mode
//
// =============================================================================

import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 1,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',

  timeout: 60_000,
  expect: {
    timeout: 10_000
  },

  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure'
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ],

  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  }
})
