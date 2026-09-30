import { test, expect } from '@playwright/test';

/**
 * Desktop topbar layout — rendered-layout verification.
 *
 * The desktop shell (Wails/webview) serves the same templates and CSS as the
 * browser build, so a real browser engine at the same 1440x900 window size is
 * a faithful stand-in for asserting *computed* layout. Unlike the Go shell
 * tests, which assert markup presence, this asserts what the user actually
 * sees: the nav is a full-width bar pinned to the top edge, and the sidebar
 * occupies no space at all.
 */

const TOPBAR_PAGES = [
  ['Dashboard', '/dashboard'],
  ['Manajemen Arsip', '/arsip'],
  ['Layanan', '/peminjaman'],
  ['Master Data', '/unit-kerja'],
  ['Laporan', '/laporan'],
  ['Administrasi (Lainnya)', '/roles'],
];

const MENU_COUNTS = {
  'Manajemen Arsip': 4,
  'Layanan': 2,
  'Master Data': 5,
  'Laporan': 10,
  'Lainnya': 8,
};

const BAR_H = 64; // --topbar-h in enterprise.css

test.describe('desktop topbar', () => {
  test('nav is a full-width bar pinned to the top, sidebar takes no space', async ({ page }) => {
    for (const [label, path] of TOPBAR_PAGES) {
      const res = await page.goto(path);
      expect(res.status(), `${label} ${path} status`).toBe(200);

      // Layout mode marker drives the CSS.
      await expect(page.locator('body')).toHaveClass(/layout-topbar/);

      // Strongest possible guarantee: in topbar mode layouts/app.html does not
      // render the sidebar at all, so it is absent from the DOM rather than
      // merely hidden, and therefore cannot take any layout space.
      const sidebarCount = await page.locator('#sidebarNav').count();
      expect(sidebarCount, `${label}: sidebar not rendered at all`).toBe(0);
      const navFootprint = await page.evaluate(() =>
        document.getElementById('sidebarNav')?.getBoundingClientRect().width ?? 0);
      expect(navFootprint, `${label}: no sidebar footprint`).toBeLessThan(1);

      // The header spans the full window; the module bar sits inside it, after
      // the brand, in a single row near the top edge.
      const header = await page.locator('.top-navbar').evaluate((el) => {
        const r = el.getBoundingClientRect();
        return { x: r.x, width: r.width };
      });
      expect(header.x, `${label}: header at left edge`).toBeLessThan(2);
      expect(header.width, `${label}: header spans the window`)
        .toBeGreaterThan(page.viewportSize().width * 0.98);

      const topnav = page.locator('#topnav');
      await expect(topnav).toBeVisible();
      const bar = await topnav.boundingBox();
      expect(bar, `${label}: topnav has a box`).toBeTruthy();
      expect(bar.y, `${label}: pinned near the top edge`).toBeLessThan(BAR_H);
      expect(bar.height, `${label}: single-row height`).toBeLessThanOrEqual(BAR_H);

      // Nothing in the bar is scrolled out of sight: the bar is exactly as wide
      // as its content, so no group can hide beyond the right edge.
      const clipped = await page.locator('#topnav').evaluate(
        (el) => el.scrollWidth - el.clientWidth);
      expect(clipped, `${label}: no group clipped by the bar`).toBeLessThanOrEqual(1);

      // The last group must be fully inside the bar's visible width.
      const lastInside = await page.locator('#topnav .topnav-item').last().evaluate((el) => {
        const nav = el.closest('#topnav');
        const a = el.getBoundingClientRect();
        const b = nav.getBoundingClientRect();
        return a.right <= b.right + 1;
      });
      expect(lastInside, `${label}: last group fully visible`).toBeTruthy();

      // Content is not pushed sideways by a sidebar.
      const main = page.locator('main, .main-content, #mainContent').first();
      if (await main.count()) {
        const mb = await main.boundingBox();
        if (mb) expect(mb.x, `${label}: content starts at left edge`).toBeLessThan(120);
      }
    }
  });

  test('exactly 5 primary groups + Lainnya, nothing wraps', async ({ page }) => {
    await page.goto('/dashboard');
    const items = page.locator('#topnav > .topnav-item, #topnav > .dropdown > .topnav-item');
    await expect(items).toHaveCount(6);

    const labels = await items.allInnerTexts();
    const clean = labels.map((t) => t.replace(/\s+/g, ' ').trim());
    expect(clean[0]).toBe('Dashboard');
    for (const want of ['Manajemen Arsip', 'Layanan', 'Master Data', 'Laporan', 'Lainnya']) {
      expect(clean.some((t) => t.startsWith(want)), `group ${want} present in ${JSON.stringify(clean)}`).toBeTruthy();
    }

    // Single row: every trigger on the same baseline, inside one bar height.
    const boxes = await items.evaluateAll((els) =>
      els.map((e) => e.getBoundingClientRect().top + window.scrollY));
    expect(new Set(boxes.map((b) => Math.round(b))).size, 'all triggers share one row').toBe(1);

    // No horizontal overflow of the bar.
    const overflow = await page.evaluate(() => {
      const n = document.getElementById('topnav');
      return n.scrollWidth - n.clientWidth;
    });
    expect(overflow, 'topbar does not overflow').toBeLessThanOrEqual(1);
  });

  for (const [group, expected] of Object.entries(MENU_COUNTS)) {
    test(`dropdown "${group}" opens with ${expected} destinations`, async ({ page }) => {
      await page.goto('/dashboard');
      const trigger = page.locator(`#topnav .topnav-item[title="${group}"]`);
      await expect(trigger).toBeVisible();
      await trigger.click();

      const menu = page.locator(`#topnav .topnav-menu:visible`).first();
      await expect(menu).toBeVisible();
      const links = menu.locator('a.dropdown-item');
      const n = await links.count();
      expect(n, `${group} item count`).toBe(expected);

      // Every destination is a real internal link.
      const hrefs = await links.evaluateAll((els) => els.map((e) => e.getAttribute('href')));
      for (const h of hrefs) expect(h, 'href present').toMatch(/^\/[a-z0-9\-\/]+$/i);

      // The menu is actually on screen and not clipped off the viewport.
      const mb = await menu.boundingBox();
      expect(mb.x).toBeGreaterThanOrEqual(-1);
      expect(mb.x + mb.width).toBeLessThanOrEqual(page.viewportSize().width + 1);
    });
  }

  test('active state marks the current group', async ({ page }) => {
    await page.goto('/laporan');
    const active = page.locator('#topnav .topnav-item.active');
    await expect(active).toHaveCount(1);
    await expect(active).toHaveAttribute('title', 'Laporan');
  });

  // The header must degrade in a fixed order -- search yields first, then the
  // labels -- and must never clip a group, at any realistic window size.
  // [width, height, labelsShown, searchShown]
  for (const [w, h, labels, search] of [
    [1920, 1080, true, true],
    [1440, 900, true, true],    // default Wails window
    [1366, 768, true, true],    // search still gets ~200px here
    [1280, 800, true, false],   // below 1340 the search steps aside
    [1100, 800, false, false],  // below 1170 labels collapse to icons
    [900, 700, false, false],
  ]) {
    test(`no clipped group at ${w}x${h}`, async ({ page }) => {
      await page.setViewportSize({ width: w, height: h });
      await page.goto('/dashboard');

      const state = await page.evaluate(() => {
        const nav = document.getElementById('topnav');
        const items = [...nav.querySelectorAll('.topnav-item')];
        const navBox = nav.getBoundingClientRect();
        const last = items[items.length - 1].getBoundingClientRect();
        const header = document.querySelector('.top-navbar').getBoundingClientRect();
        const search = document.querySelector('.nav-search');
        return {
          clipped: nav.scrollWidth - nav.clientWidth,
          lastVisible: last.right <= navBox.right + 1,
          rows: new Set(items.map((e) => Math.round(e.getBoundingClientRect().top))).size,
          labelsShown: getComputedStyle(items[1].querySelector('span')).display !== 'none',
          searchShown: search ? getComputedStyle(search).display !== 'none' : false,
          searchWidth: search ? Math.round(search.getBoundingClientRect().width) : 0,
          headerWidth: header.width,
          headerX: header.x,
        };
      });

      expect(state.clipped, `${w}px: nothing clipped`).toBeLessThanOrEqual(1);
      expect(state.lastVisible, `${w}px: last group visible`).toBeTruthy();
      expect(state.rows, `${w}px: bar stays on one row`).toBe(1);
      expect(state.labelsShown, `${w}px: label visibility`).toBe(labels);
      expect(state.searchShown, `${w}px: search visibility`).toBe(search);
      expect(state.headerX, `${w}px: header at left edge`).toBeLessThan(2);
      expect(state.headerWidth, `${w}px: header fills the window`)
        .toBeGreaterThan(w * 0.98);

      // A search box that is on screen must also be big enough to type in;
      // otherwise it should have stepped aside at this width.
      if (state.searchShown) {
        expect(state.searchWidth, `${w}px: visible search is usable`)
          .toBeGreaterThanOrEqual(150);
      }
    });
  }
});
