// Login admin SEKALI lalu simpan storageState untuk dipakai semua test,
// sehingga rate-limiter login tidak pernah tersentuh oleh suite.
import { test as setup } from '@playwright/test';

const AUTH_FILE = '.auth/admin.json';

setup('autentikasi admin → simpan sesi', async ({ page }) => {
  await page.goto('/login');
  await page.fill('input[name="username"]', process.env.TEST_USER || 'admin');
  await page.fill('input[name="password"]', process.env.TEST_PASS || 'admin');
  await page.click('button[type="submit"]');
  await page.waitForSelector('.sidebar, .top-navbar, .mobile-bottom-nav', { timeout: 60000 });
  await page.context().storageState({ path: AUTH_FILE });
});