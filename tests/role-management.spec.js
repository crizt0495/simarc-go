import { test, expect } from '@playwright/test';

/**
 * SIMARC Manajemen Role — uji CRUD lengkap hingga 100% sukses.
 *
 * Alur yang diverifikasi:
 *   1. Index /roles render bersih (200, nol error konsol).
 *   2. Tambah role (Create → Store → muncul di tabel + badge Custom).
 *   3. Detail role (Show: judul, statistik hak akses & pengguna).
 *   4. Ubah role (Edit → Update → nama tampil di tabel).
 *   5. Kelola hak akses (Permissions: pilih semua → simpan → persist).
 *   6. Perlindungan role sistem Admin (checkbox/save terkunci, POST ditolak).
 *   7. Hapus role uji (konfirmasi modal → terhapus dari tabel).
 *
 * Menjalankan: `npx playwright test tests/role-management.spec.js`
 */

// State bersama antar subtest (per worker = per project chromium/mobile).
const created = { id: null, suffix: '', namaRole: '', technicalName: '' };

/** Kumpulkan error konsol/page (header/body tidak dianggap instruksi). */
async function probe(page) {
  const errors = [];
  page.on('pageerror', e => errors.push('PAGE: ' + String(e).slice(0, 160)));
  page.on('console', m => {
    if (m.type() === 'error') errors.push('CONSOLE: ' + m.text().slice(0, 160));
  });
  await page.waitForTimeout(250);
  page.removeAllListeners('pageerror');
  return errors.filter(e => !e.includes('Failed to load resource'));
}

/** Tunggu modal flash SIMARC V4 dan klaim isinya, lalu tutup. */
async function expectFlash(page, title, message) {
  const flash = page.locator('#sm-portal');
  await expect(flash.locator('.sm-confirm-message')).toBeVisible({ timeout: 10000 });
  if (title) {
    await expect(flash.locator('.sm-confirm-title')).toHaveText(title);
  }
  if (message) {
    await expect(flash.locator('.sm-confirm-message')).toHaveText(message);
  }
  const close = flash.locator('.sm-btn-cancel');
  if ((await close.count()) > 0) {
    await close.click({ timeout: 3000 }).catch(() => {});
  }
}

/** Ambil CSRF token dari meta di halaman aktif. */
async function pageCSRF(page) {
  return await page.evaluate(() => {
    const m = document.querySelector('meta[name="csrf-token"]');
    return m ? m.getAttribute('content') : '';
  });
}

/** Ambil baris role di tabel index berdasarkan teks nama tampilan. */
function roleRow(page, text) {
  return page.locator('#roleTable tbody tr', { hasText: text }).first();
}

test.describe.serial('Manajemen Role — CRUD + Hak Akses', () => {
  test.setTimeout(120000);

  test.afterAll(async ({ request }) => {
    // Bersihkan role sisa bila subtest gagal di tengah alur (jangan menumpuk data).
    // Rute auth tidak memvalidasi CSRF, jadi cukup POST langsung tanpa token.
    if (!created.id || !created.suffix) return;
    await request.post(`/roles/${created.id}/delete`, { maxRedirects: 0 }).catch(() => {});
  });

  test('01 · index: daftar role render 200 tanpa error konsol', async ({ page }) => {
    const resp = await page.goto('/roles', { waitUntil: 'domcontentloaded' });
    expect(resp.status()).toBe(200);
    await expect(page.locator('#roleTable')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('h1.hero-title')).toContainText('Kontrol Akses & Keamanan');
    // Kartu KPI dan baris role sistem tersedia
    await expect(page.locator('.kpi-role-card').first()).toBeVisible();
    const adminRow = roleRow(page, 'Admin');
    await expect(adminRow).toBeVisible();
    await expect(adminRow.locator('.role-ident')).toHaveText(/System/);
    const errors = await probe(page);
    expect(errors, errors.join('\n')).toEqual([]);
  });

  test('02 · tambah role baru', async ({ page }) => {
    const resp = await page.goto('/roles/create', { waitUntil: 'domcontentloaded' });
    expect(resp.status()).toBe(200);
    await expect(page.locator('#createRoleForm')).toBeVisible({ timeout: 10000 });

    created.suffix = Date.now().toString(36);
    created.technicalName = 'role-test-' + created.suffix;
    created.namaRole = 'Role Uji ' + created.suffix;

    await page.fill('#name', created.technicalName);
    await page.fill('#nama_role', created.namaRole);
    await page.fill('#keterangan', 'Role dibuat oleh uji otomatis manajemen role.');
    await page.click('#createRoleForm button[type="submit"]');

    await page.waitForURL(/\/roles(\?.*)?$/, { timeout: 15000 });
    await expectFlash(page, 'Berhasil', 'Role berhasil ditambahkan.');

    const row = roleRow(page, created.namaRole);
    await expect(row).toBeVisible({ timeout: 10000 });
    await expect(row.locator('.role-ident')).toHaveText(/Custom/);

    const editHref = await row.locator('a.btn-crud-edit').getAttribute('href');
    const id = editHref.split('/')[2];
    expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
    created.id = id;

    const errors = await probe(page);
    expect(errors, errors.join('\n')).toEqual([]);
  });

  test('03 · detail role', async ({ page }) => {
    await page.goto(`/roles/${created.id}`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('h1.rph-title')).toContainText(created.namaRole, { timeout: 10000 });
    await expect(page.locator('.rph-stat-value')).toHaveCount(2);
    await expect(page.locator('.rph-stat-value').first()).toHaveText('0'); // Hak Akses
    await expect(page.locator('.rph-stat-label').first()).toHaveText('Hak Akses');
    await expect(page.locator('.rph-stat-label').last()).toHaveText('Pengguna');
    const errors = await probe(page);
    expect(errors, errors.join('\n')).toEqual([]);
  });

  test('04 · ubah role', async ({ page }) => {
    await page.goto(`/roles/${created.id}/edit`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('#nama_role')).toHaveValue(created.namaRole, { timeout: 10000 });
    await expect(page.locator('#name')).toHaveValue(created.technicalName);

    const namaBaru = created.namaRole + ' (Diubah)';
    await page.fill('#nama_role', namaBaru);
    await page.fill('#keterangan', 'Deskripsi diperbarui oleh uji otomatis.');
    await page.click('.edit-role-card form button[type="submit"]');

    await page.waitForURL(/\/roles(\?.*)?$/, { timeout: 15000 });
    await expectFlash(page, 'Berhasil', 'Role berhasil diperbarui.');
    created.namaRole = namaBaru;

    const row = roleRow(page, namaBaru);
    await expect(row).toBeVisible({ timeout: 10000 });
    await expect(row.locator('td').nth(1)).toContainText(namaBaru);

    const errors = await probe(page);
    expect(errors, errors.join('\n')).toEqual([]);
  });

  test('05 · kelola hak akses (simpan & persistensi)', async ({ page }) => {
    await page.goto(`/roles/${created.id}/permissions`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('#permForm')).toBeVisible({ timeout: 10000 });

    // Role baru: belum ada permission terpilih.
    const wrap = page.locator('#checkedCountWrap');
    await expect(wrap).toContainText('dari', { timeout: 10000 });
    const wrapText = await wrap.textContent();
    const total = parseInt(wrapText.split('dari')[1].trim(), 10);
    expect(total).toBeGreaterThan(0);

    // Pilih semua → counter & badge berubah.
    await page.locator('#globalSelectAll').scrollIntoViewIfNeeded();
    await page.locator('#globalSelectAll').click();
    await expect(page.locator('#checkedCount')).toHaveText(String(total));
    await expect(page.locator('#saveCount')).toHaveText(String(total));

    // Filter pencarian bekerja (sembunyikan yang tak cocok).
    await page.fill('#permSearch', 'dashboard');
    await page.waitForTimeout(300);
    const visibleCols = await page.locator('.perm-col:not(.hidden-by-search)').count();
    expect(visibleCols).toBeGreaterThan(0);
    await page.fill('#permSearch', '');
    await page.waitForTimeout(300);

    // Simpan.
    await page.locator('#saveBtn').scrollIntoViewIfNeeded();
    await page.locator('#saveBtn').click();
    await page.waitForURL(/\/permissions$/, { timeout: 15000 });
    await expectFlash(page, 'Berhasil', 'Hak akses berhasil diperbarui.');

    // Persistensi: buka lagi → pilihan tersimpan.
    await page.goto(`/roles/${created.id}/permissions`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('#checkedCount')).toHaveText(String(total), { timeout: 10000 });
    const checked = await page.locator('input[name="permissions[]"]:checked').count();
    expect(checked).toBe(total);

    const errors = await probe(page);
    expect(errors, errors.join('\n')).toEqual([]);
  });

  test('06 · perlindungan role sistem (Admin)', async ({ page }) => {
    await page.goto('/roles', { waitUntil: 'domcontentloaded' });
    await expect(page.locator('#roleTable')).toBeVisible({ timeout: 10000 });

    const adminRow = roleRow(page, 'Admin');
    const deleteBtn = adminRow.locator('.btn-crud-del');
    await expect(deleteBtn).toBeDisabled();
    await expect(deleteBtn.locator('i')).toHaveClass(/bi-lock/);

    const editHref = await adminRow.locator('a.btn-crud-edit').getAttribute('href');
    const adminId = editHref.split('/')[2];

    // Halaman permission Admin: semua terkunci + alert konfigurasi.
    await page.goto(`/roles/${adminId}/permissions`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('.admin-alert')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('input[name="permissions[]"]').first()).toBeDisabled();
    await expect(page.locator('#saveBtn')).toBeDisabled();

    // POST langsung ke UpdatePermissions → ditolak. maxRedirects:0 agar
// redirect respons tidak ikut dikonsumsi flash-nya.
    const csrf = await pageCSRF(page);
    await page.request.post(`/roles/${adminId}/permissions`, { headers: { 'X-CSRF-Token': csrf }, maxRedirects: 0 });
    await page.goto(`/roles/${adminId}/permissions`, { waitUntil: 'domcontentloaded' });
    await expectFlash(page, 'Gagal', 'Tidak dapat mengubah permission role Admin.');

    // POST langsung ke Destroy → role sistem tidak bisa dihapus.
    await page.request.post(`/roles/${adminId}/delete`, { headers: { 'X-CSRF-Token': csrf }, maxRedirects: 0 });
    await page.goto('/roles', { waitUntil: 'domcontentloaded' });
    await expectFlash(page, 'Gagal', 'Tidak dapat menghapus role sistem.');
    await expect(adminRow.locator('.btn-crud-del')).toBeDisabled();
  });

  test('07 · hapus role uji', async ({ page }) => {
    await page.goto('/roles', { waitUntil: 'domcontentloaded' });
    const row = roleRow(page, created.namaRole);
    await expect(row).toBeVisible({ timeout: 10000 });

    const delBtn = row.locator('.btn-crud-del:not([disabled])');
    await expect(delBtn).toBeEnabled();
    await delBtn.scrollIntoViewIfNeeded();
    await delBtn.click();

    // Modal konfirmasi SIMARC V4.
    const modal = page.locator('#sm-portal');
    await expect(modal).toContainText('Hapus Role?', { timeout: 10000 });
    await expect(modal.locator('#deleteRoleName')).toHaveText(created.namaRole);
    await modal.locator('form#deleteForm .sm-btn-danger').click();

    await page.waitForURL(/\/roles(\?.*)?$/, { timeout: 15000 });
    await expectFlash(page, 'Berhasil', 'Role berhasil dihapus.');
    await expect(row).toHaveCount(0);
    created.id = null; // sudah dibersihkan — afterAll tidak perlu apa-apa

    const errors = await probe(page);
    expect(errors, errors.join('\n')).toEqual([]);
  });
});