import { defineConfig } from '@playwright/test';

/**
 * Desktop topbar layout checks, aimed at an already-running engine.
 *
 *   SIMARC_LAYOUT=topbar PORT=8099 go run ./cmd/server
 *   npx playwright test -c playwright.topbar.config.js
 *
 * Reuses the admin session saved by tests/auth.setup.spec.js. The viewport
 * matches the Wails window so computed layout is representative.
 */
export default defineConfig({
  testDir: './tests',
  testMatch: /desktop-topbar\.spec\.js/,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8099',
    viewport: { width: 1440, height: 900 },
    storageState: '.auth/admin.json',
  },
});
