import { test, expect } from '@playwright/test';

/**
 * SIMARC UI Smoke — setiap halaman render 200, nol error konsol,
 * tanpa overflow horizontal; interaksi inti berfungsi.
 * Menjalankan per title: `npx playwright test tests/ui-smoke.spec.js`.
 */

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

/** Kumpulkan error konsol/page + cek overflow, kembalikan laporan. */
async function probe(page) {
  const errors = [];
  page.on('pageerror', e => errors.push('PAGE: ' + String(e).slice(0, 160)));
  page.on('console', m => {
    if (m.type() === 'error') errors.push('CONSOLE: ' + m.text().slice(0, 160));
  });
  await page.waitForTimeout(250);
  const { docW, vw } = await page.evaluate(() => ({
    docW: document.documentElement.scrollWidth,
    vw: window.innerWidth,
  }));
  page.removeAllListeners('pageerror');
  const clean = errors.filter(e => !e.includes('Failed to load resource'));
  return { errors: clean, hOverflow: docW > vw + 1 };
}

const MAIN_ROUTES = [
  '/dashboard', '/arsip', '/jenis-arsip', '/kode-klasifikasi', '/lokasi-arsip',
  '/unit-kerja', '/users', '/roles', '/peminjaman', '/pemberkasan',
  '/pemusnahan', '/disposal', '/jadwal-retensi', '/pengaturan',
  '/pengaturan/system', '/profil', '/advanced/integrations', '/blockchain',
  '/settings', '/laporan', '/laporan/arsip',
  '/advanced/import-export', '/backup', '/monitoring/retensi', '/ocr', '/search',
];

test.setTimeout(360000);
test('semua halaman utama: 200, nol error konsol, tanpa overflow', async ({ page }) => {
  await login(page);
  const failures = [];
  for (const route of MAIN_ROUTES) {
    const resp = await page.goto(route, { waitUntil: 'domcontentloaded' });
    const report = await probe(page);
    if (resp.status() !== 200) failures.push(`${route} → HTTP ${resp.status()}`);
    if (report.errors.length) failures.push(`${route} → ${report.errors.join(' | ')}`);
    if (report.hOverflow) failures.push(`${route} → overflow horizontal ${report.hOverflow}`);
  }
  expect(failures, failures.join('\n')).toEqual([]);
});

test('arsip: toggle tampilan kartu ↔ tabel berfungsi', async ({ page }) => {
  await login(page);
  await page.goto('/arsip', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(600);
  const cardBtn = page.locator('.view-toggle-btn[data-view="card"]').first();
  const tableBtn = page.locator('.view-toggle-btn[data-view="table"]').first();
  await expect(tableBtn).toBeVisible();
  await tableBtn.click();
  await page.waitForTimeout(600);
  const report = await probe(page);
  expect(report.errors, report.errors.join('\n')).toEqual([]);
  expect(report.hOverflow).toBe(false);
  await cardBtn.click();
  await page.waitForTimeout(500);
  const report2 = await probe(page);
  expect(report2.errors, report2.errors.join('\n')).toEqual([]);
});

test('arsip: panel filter collapsible', async ({ page }) => {
  await login(page);
  await page.goto('/arsip', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(500);
  const trigger = page.locator('[data-bs-target="#arsipFilterCollapse"]').first();
  if ((await trigger.count()) > 0) {
    await trigger.click();
    await page.waitForTimeout(500);
    await trigger.click();
    const report = await probe(page);
    expect(report.errors, report.errors.join('\n')).toEqual([]);
  }
});

test('dashboard: tombol perbarui data bekerja tanpa error', async ({ page }) => {
  await login(page);
  const btn = page.locator('#btnRefresh');
  await expect(btn).toBeVisible();
  await btn.scrollIntoViewIfNeeded();
  await btn.click({ timeout: 30000 });
  await page.waitForTimeout(1500);
  const report = await probe(page);
  expect(report.errors, report.errors.join('\n')).toEqual([]);
  expect(report.hOverflow).toBe(false);
});

test('form create tersedia di jalur CRUD utama', async ({ page }) => {
  await login(page);
  for (const route of ['/arsip/create', '/jenis-arsip/create', '/kode-klasifikasi/create', '/unit-kerja/create']) {
    const resp = await page.goto(route, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(500);
    const report = await probe(page);
    const submitCount = await page.locator('button[type="submit"], .btn-primary').count();
    expect(resp.status(), `${route} status`).toBe(200);
    expect(report.errors, `${route} errors: ${report.errors.join(' | ')}`).toEqual([]);
    expect(submitCount, `${route} harus punya tombol submit`).toBeGreaterThan(0);
  }
});

test('pencarian: hasil tampil tanpa error', async ({ page }) => {
  await login(page);
  const resp = await page.goto('/search?q=arsip', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(600);
  const report = await probe(page);
  expect(resp.status()).toBe(200);
  expect(report.errors, report.errors.join('\n')).toEqual([]);
  expect(report.hOverflow).toBe(false);
});