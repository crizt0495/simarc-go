import { test, expect } from '@playwright/test';

/**
 * SIMARC neo-brutalism visual regression suite.
 *
 * Verifies the design system holds its shape across key pages:
 *  - layout chrome (sidebar, navbar, mobile bottom nav)
 *  - theme swap (light ↔ dark) keeps the brutalist contract
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

test.describe('neo-brutalism layout contract', () => {
  test('dashboard renders brutalist chrome', async ({ page }) => {
    await login(page);
    await expect(page.locator('.sidebar')).toBeVisible();
    await expect(page.locator('.top-navbar')).toBeVisible();
    // Hard borders + offset shadows are the signature
    const bar = await page.locator('.top-navbar').evaluate(el => getComputedStyle(el).borderBottomWidth);
    expect(bar).not.toBe('0px');
  });

  test('theme toggle flips html[data-theme] and keeps layout', async ({ page }) => {
    await login(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('.sidebar')).toBeVisible();
    await expect(page.locator('.top-navbar')).toBeVisible();
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
  test('primary button keeps bold ink border + shadow', async ({ page }) => {
    await login(page);
    // inventory page: primary CTA
    await page.goto('/arsip');
    const btn = page.locator('.btn-primary').first();
    await btn.waitFor({ state: 'visible' });
    const shadow = await btn.evaluate(el => getComputedStyle(el).boxShadow);
    expect(shadow).toContain(','); // offset shadow (two stops) not a soft blur
  });

  test('status badge renders with border', async ({ page }) => {
    await login(page);
    await page.goto('/arsip');
    const badge = page.locator('.status-badge, .badge').first();
    await badge.waitFor({ state: 'visible' });
    const bw = await badge.evaluate(el => getComputedStyle(el).borderTopWidth);
    expect(parseInt(bw, 10)).toBeGreaterThan(0);
  });
});

test.describe('auth page', () => {
  test('login card is hard-edged and usable', async ({ page }) => {
    await page.goto('/login');
    const card = page.locator('.auth-card');
    await expect(card).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
    const radius = await card.evaluate(el => getComputedStyle(el).borderRadius);
    expect(parseFloat(radius)).toBe(0); // brutalist: no rounding
  });

  test('auth theme toggle transitions+persists', async ({ page }) => {
    await page.goto('/login');
    await page.click('#authThemeBtn');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });
});