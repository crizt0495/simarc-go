import { test, expect } from '@playwright/test';

/**
 * Topbar dropdowns must be visible, not merely present.
 *
 * Each group in the top bar is a Bootstrap dropdown, and its menu is a child of
 * the bar itself. So the moment the bar gets any overflow, the menus are clipped
 * inside it: the menu still gets a full width and height, still receives
 * `aria-expanded="true"` and the `show` class, and the click "works" — but
 * nothing is painted below the bar, so from the user's side the submenu simply
 * never appears.
 *
 * That is why these tests assert what is painted rather than what exists. An
 * earlier version of this suite checked only that the menu opened and had
 * non-zero size, and passed while the bug was live.
 */

const GROUPS = [
  { name: 'Manajemen Arsip', items: 4 },
  { name: 'Layanan', items: 2 },
  { name: 'Master Data', items: 5 },
  { name: 'Laporan', items: 10 },
  { name: 'Lainnya', items: 8 },
];

const trigger = (page, name) =>
  page.locator('#topnav .topnav-item', { hasText: name }).first();

/** Menu geometry plus whether the menu is actually painted on screen. */
const menuState = (page, name) =>
  page.evaluate((label) => {
    const el = [...document.querySelectorAll('#topnav .topnav-item')]
      .find((n) => (n.textContent || '').trim().slice(0, 20) === label);
    const menu = el.parentNode.querySelector('.topnav-menu');
    const r = menu.getBoundingClientRect();

    // Hit-test the middle of the menu. If an ancestor is clipping it away, this
    // returns something outside the menu instead of one of its own items.
    const at = document.elementFromPoint(
      Math.round(r.left + r.width / 2),
      Math.round(r.top + Math.min(r.height, 40) / 2)
    );
    const painted = !!(at && (menu.contains(at) || at === menu));

    // Any ancestor smaller than the menu that also clips is the culprit.
    const clippers = [];
    for (let n = menu.parentElement; n && n !== document.documentElement; n = n.parentElement) {
      const cs = getComputedStyle(n);
      const nr = n.getBoundingClientRect();
      if (
        (cs.overflowY !== 'visible' && nr.height < r.height) ||
        (cs.overflowX !== 'visible' && nr.width < r.width)
      ) {
        clippers.push({
          sel: (n.id ? '#' + n.id : n.tagName.toLowerCase() + '.' +
                String(n.className || '').trim().split(/\s+/).join('.')).slice(0, 60),
          overflowX: cs.overflowX,
          overflowY: cs.overflowY,
          box: { w: Math.round(nr.width), h: Math.round(nr.height) },
        });
      }
    }

    return {
      width: Math.round(r.width),
      height: Math.round(r.height),
      top: Math.round(r.top),
      bottom: Math.round(r.bottom),
      inViewport: r.top >= 0 && r.bottom <= window.innerHeight,
      painted,
      clippers,
      items: menu.querySelectorAll('.dropdown-item').length,
    };
  }, name);

test.describe('topbar dropdown menus are visible', () => {
  for (const { name, items } of GROUPS) {
    test(`${name} menu is painted below the bar, not clipped by it`, async ({ page }) => {
      await page.goto('/dashboard');
      await expect(page.locator('#topnav')).toBeVisible();

      await trigger(page, name).click();
      const menu = page.locator(`#topnav .topnav-menu.show`).first();
      await expect(menu).toBeVisible();

      const state = await menuState(page, name);

      // The regression: the bar must not clip its own menus.
      expect(
        state.clippers,
        `${name} menu is clipped by ${JSON.stringify(state.clippers)}`
      ).toEqual([]);

      // A menu that is "open" but not painted is the exact user-visible bug.
      expect(state.painted, `${name} menu opens but nothing is painted`).toBe(true);
      expect(state.inViewport, `${name} menu is outside the viewport`).toBe(true);

      // The whole menu fits, not just its first row.
      expect(state.height).toBeGreaterThan(40);
      expect(state.items).toBe(items);
    });
  }

  test('the bar itself never establishes a scroll or clip context', async ({ page }) => {
    // Catching this at the source is cheaper than catching five clipped menus:
    // the bar holds the menus, so any overflow on it truncates all of them.
    await page.goto('/dashboard');
    const overflow = await page.evaluate(() => {
      const bar = document.getElementById('topnav');
      const cs = getComputedStyle(bar);
      return { overflowX: cs.overflowX, overflowY: cs.overflowY };
    });
    expect(overflow.overflowX).toBe('visible');
    expect(overflow.overflowY).toBe('visible');
  });

  test('menus stay fully visible at every supported window width', async ({ page }) => {
    for (const width of [1920, 1440, 1366, 1280, 1100, 1024]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto('/dashboard');
      await trigger(page, 'Laporan').click();
      const state = await menuState(page, 'Laporan');
      expect(state.painted, `Laporan menu not painted at ${width}px`).toBe(true);
      expect(state.clippers, `Laporan menu clipped at ${width}px`).toEqual([]);
      await page.keyboard.press('Escape');
    }
  });
});
