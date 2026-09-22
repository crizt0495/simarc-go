import { defineConfig } from '@playwright/test';

/**
 * SIMARC Visual Regression — Playwright.
 *
 * Running against the production build ensures the enterprise design
 * system renders consistently. First run: `npx playwright test --update-snapshots`.
 * Requires: `npm i -D @playwright/test && npx playwright install chromium`.
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Server dev/db tunggal: jalankan serial agar tidak membanjiri koneksi.
  workers: process.env.CI ? undefined : 1,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    viewport: { width: 1440, height: 900 },
  },
  projects: [
    // Login sekali → semua proyek memakai sesi yang sama (hindari rate-limit login).
    { name: 'setup', testMatch: /auth\.setup\.spec\.js/ },
    {
      name: 'chromium',
      use: { browserName: 'chromium', storageState: '.auth/admin.json' },
      testIgnore: /auth\.setup\.spec\.js/,
      dependencies: ['setup'],
    },
    {
      name: 'mobile',
      use: { browserName: 'chromium', viewport: { width: 390, height: 844 }, storageState: '.auth/admin.json' },
      testIgnore: /auth\.setup\.spec\.js/,
      dependencies: ['setup'],
    },
  ],
});