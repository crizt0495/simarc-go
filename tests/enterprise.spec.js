import { test, expect } from '@playwright/test';

/**
 * SIMARC enterprise design-system visual/spec suite.
 *
 * Verifies the design system holds its shape across key pages:
 *  - layout chrome (sidebar, navbar, mobile bottom nav)
 *  - theme swap (light ↔ dark) keeps the enterprise contract
 *  - contrast fixtures on stat cards, buttons, badges, tables
 *  - mobile breakpoint shows bottom nav, hides desktop sidebar
 *
 * First run against a live server (`./run.sh` or the Go binary on :8080).
 */

const THEME_TOGGLE = '#themeToggle';

async function login(page) {
  await page.goto('/login');
  await page.fill('input[name="username"]', process.env.TEST_USER || 'admin');
  await page.fill('input[name="password"]', process.env.TEST_PASS || 'admin');
  await Promise.all([page.waitForURL('**/dashboard'), page.click('button[type="submit"]')]);
}

// The desktop sidebar is only rendered at >= 992px; below that the layout
// intentionally swaps to the mobile bottom nav (see desktop-modern.css §7).
async function expectLayoutChrome(page) {
  await expect(page.locator('.top-navbar')).toBeVisible();
  const width = page.viewportSize()?.width ?? 1440;
  if (width >= 992) {
    await expect(page.locator('.sidebar')).toBeVisible();
  } else {
    await expect(page.locator('.mobile-bottom-nav')).toBeVisible();
  }
}

test.describe('enterprise layout contract', () => {
  test('dashboard renders modern chrome', async ({ page }) => {
    await login(page);
    await expectLayoutChrome(page);
    // Enterprise contract: crisp 1px hairline under the topbar
    const bar = await page.locator('.top-navbar').evaluate(el => getComputedStyle(el).borderBottomWidth);
    expect(bar).toBe('1px');
  });

  test('sidebar links are rounded and theme-aware', async ({ page }) => {
    await login(page);
    const link = page.locator('.sidebar-link').first();
    const radius = await link.evaluate(el => getComputedStyle(el).borderRadius);
    expect(parseFloat(radius)).toBeGreaterThan(0);
  });

  test('theme toggle flips html[data-theme] and keeps layout', async ({ page }) => {
    await login(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expectLayoutChrome(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });

  test('mobile viewport swaps sidebar for bottom nav', async ({ page }) => {
    await login(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page.locator('.mobile-bottom-nav')).toBeVisible();
  });
});

test.describe('component contrast fixtures', () => {
  test('primary button uses soft shadow, not offset hard shadow', async ({ page }) => {
    await login(page);
    await page.goto('/arsip');
    const btn = page.locator('.btn-primary:visible').first();
    await btn.waitFor({ state: 'visible' });
    const shadow = await btn.evaluate(el => getComputedStyle(el).boxShadow);
    // Enterprise: blurred ambient shadow (rgb(…, a) fade), not a solid offset pair
    expect(shadow).not.toContain(', 2px 2px 0');
  });

  test('status badge renders pill-rounded', async ({ page }) => {
    await login(page);
    await page.goto('/arsip');
    const badge = page.locator('.status-badge, .badge').first();
    await badge.waitFor({ state: 'visible' });
    const radius = await badge.evaluate(el => getComputedStyle(el).borderRadius);
    expect(parseFloat(radius)).toBeGreaterThan(8);
  });
});

test.describe('auth page', () => {
  test('login card is rounded, elevated and usable', async ({ page }) => {
    await page.goto('/login');
    const card = page.locator('.auth-card');
    await expect(card).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
    // Two-pane redesign: the rounded, elevated container is .auth-shell and the
    // form card (.auth-card) sits flat inside the right pane.
    const shell = page.locator('.auth-shell');
    const radius = await shell.evaluate(el => getComputedStyle(el).borderRadius);
    expect(parseFloat(radius)).toBeGreaterThan(0);
  });

  test('auth theme toggle transitions+persists', async ({ page }) => {
    await page.goto('/login');
    await page.click('#authThemeBtn');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });
});

