import { test, expect } from '@playwright/test';

/**
 * SIMARC Visual Regression — kerapian UI 100% terkunci.
 *
 * Snapshot per-pixel halaman kunci (light + dark, desktop + mobile).
 * Pertama kali: `npm run test:ui:update` (generate baseline), lalu ran
 * biasa `npm run test:ui`. Perubahan layout akan terdeteksi sebagai fail.
 *
 * Server harus berjalan di :8080 (binary Go baru dengan CSS embed terbaru).
 */

const SHOT_OPTS = { animations: 'disabled', caret: 'hide' };

async function login(page) {
  // Cek sesi dulu: GET /login tidak me-redirect user yang sudah login,
  // jadi tidak bisa dipakai sebagai penanda autentikasi.
  await page.goto('/dashboard');
  const chrome = await page.locator('.sidebar, .top-navbar, .mobile-bottom-nav').count();
  if (chrome > 0) return; // sudah punya sesi (storageState) — jangan login lagi
  await page.goto('/login');
  await page.fill('#username', process.env.TEST_USER || 'admin');
  await page.fill('#password', process.env.TEST_PASS || 'admin');
  await page.click('button[type="submit"]');
  await page.waitForSelector('.sidebar, .top-navbar, .mobile-bottom-nav', { timeout: 60000 });
}

async function settle(page, ms = 600) {
  await page.waitForLoadState('networkidle', { timeout: 10000 }).catch(() => {});
  await page.evaluate(() => document.fonts.ready).catch(() => {});
  await page.waitForTimeout(ms);
}

test('halaman login — layout & floating label', async ({ browser }) => {
  // Halaman login wajib tanpa sesi — pakai context khusus tanpa storageState.
  const ctx = await browser.newContext({ storageState: undefined, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  await page.goto('/login');
  await settle(page);
  await expect(page.locator('.auth-card')).toBeVisible();
  await expect(page.locator('label[for="username"]')).toBeVisible();
  await expect(page.locator('label[for="password"]')).toBeVisible();
  await expect(page.locator('.auth-card')).toHaveScreenshot('login.png', {
    fullPage: true,
    ...SHOT_OPTS,
  });
  await ctx.close();
});

test('dashboard — grid KPI, header, grafik', async ({ page }) => {
  await login(page);
  await settle(page, 1400); // tunggu chart + animasi selesai
  const chart = page.locator('.dash-chart-card canvas').first();
  const feed = page.locator('.dash-activity-card'); // feed aktivitas = data live → di-mask
  await expect(page.locator('.kpi-grid .stat-card').first()).toBeVisible();
  await expect(page.locator('.page-title')).toContainText('Dashboard');
  await expect(page.locator('.page-actions')).toBeVisible();
  await expect(page.locator('.main-content')).toHaveScreenshot('dashboard.png', {
    mask: [chart, feed], // chart.js + feed live di-mask agar snapshot stabil
    ...SHOT_OPTS,
  });
});

test('dashboard — mode gelap tetap rapi', async ({ page }) => {
  await login(page);
  await page.click('#themeToggle');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await settle(page, 1200);
  const chart = page.locator('.dash-chart-card canvas').first();
  const feed = page.locator('.dash-activity-card'); // feed aktivitas = data live → di-mask
  await expect(page.locator('.main-content')).toHaveScreenshot('dashboard-dark.png', {
    mask: [chart, feed],
    ...SHOT_OPTS,
  });
});

test('arsip — hero header, stat mini, daftar', async ({ page }) => {
  await login(page);
  await page.goto('/arsip');
  await settle(page, 800);
  await expect(page.locator('.hero-title')).toContainText('Daftar Arsip');
  await expect(page.locator('.stat-mini').first()).toBeVisible();
  await expect(page.locator('.main-content')).toHaveScreenshot('arsip.png', SHOT_OPTS);
});

test('pengaturan — form & layout', async ({ page }) => {
  await login(page);
  await page.goto('/pengaturan');
  await settle(page);
  await expect(page.locator('.page-title, h1').first()).toBeVisible();
  await expect(page.locator('.main-content')).toHaveScreenshot('pengaturan.png', SHOT_OPTS);
});

test('profil — identitas pengguna', async ({ page }) => {
  await login(page);
  await page.goto('/profil');
  await settle(page);
  await expect(page.locator('.page-title, h1').first()).toBeVisible();
  // Nilai "Terakhir Login" berubah tiap run (login ulang) → di-mask.
  // Mask parent (col-md-6) karena overflow tipografi inline <strong> bisa lolos
  // dari kotak bounding-box-nya sendiri (antialiasing ekor string).
  const t1 = page.locator('tr', { hasText: 'Terakhir Login' });
  const t2 = page.locator('xpath=//small[normalize-space(text())="Terakhir Login"]/ancestor::div[1]');
  await expect(page.locator('.main-content')).toHaveScreenshot('profil.png', {
    mask: [t1, t2],
    ...SHOT_OPTS,
  });
});