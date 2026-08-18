/**
 * Regression tests for the mobile momentum-scrolling CSS inside
 * web/templates/layouts/app.html (inline <style> blocks).
 *
 * Background: `.table-responsive` used to be part of the "Momentum Scrolling
 * for iOS" rule that applied `overscroll-behavior: contain` (both axes).
 * Because a wide table is a two-axis scroll container, a vertical swipe
 * starting on the table was swallowed by the table and never chained to the
 * page — on mobile the page could not be scrolled when the finger was over a
 * table (and tables dominate mobile screens).
 *
 * Fix (verified via CDP touch emulation): `.table-responsive` now only
 * contains the horizontal axis (`overscroll-behavior-x: contain`) so vertical
 * swipes propagate to the page while horizontal overscroll stays contained.
 *
 * These tests fail if that rule regresses (e.g. someone re-adds `.table-responsive`
 * to the full `overscroll-behavior: contain` group).
 */

import { describe, expect, it } from 'vitest';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
// Allow pointing at a different template (used by the negative test harness).
const APP_LAYOUT_PATH =
  process.env.APP_LAYOUT_PATH || path.resolve(__dirname, '../../../templates/layouts/app.html');
const appHtml = fs.readFileSync(APP_LAYOUT_PATH, 'utf-8');

// ─── Tiny CSS rule extractor (brace-aware; nested @media handled) ───────────

/** Return the text of every <style>…</style> block in the HTML. */
function extractStyleBlocks(html) {
  const blocks = [];
  const re = /<style[^>]*>([\s\S]*?)<\/style>/gi;
  let m;
  while ((m = re.exec(html)) !== null) blocks.push(m[1]);
  return blocks;
}

/** Remove CSS comments (they may sit in front of selectors / inside bodies). */
function stripComments(css) {
  return css.replace(/\/\*[\s\S]*?\*\//g, '');
}

/**
 * Split a CSS rule body into { selector, body } pairs with brace matching, so
 * nested at-rules (@media, @supports) collapse into one entry whose body may
 * itself contain rules. Good enough for the assertions below.
 */
function parseRules(css) {
  const rules = [];
  let i = 0;
  const len = css.length;
  while (i < len) {
    if (css.startsWith('/*', i)) {
      const end = css.indexOf('*/', i + 2);
      i = end === -1 ? len : end + 2;
      continue;
    }
    const open = css.indexOf('{', i);
    if (open === -1) break;
    let depth = 0;
    let close = -1;
    for (let j = open; j < len; j++) {
      if (css[j] === '{') depth++;
      else if (css[j] === '}') {
        depth--;
        if (depth === 0) { close = j; break; }
      }
    }
    if (close === -1) break;
    const selector = stripComments(css.slice(i, open)).trim();
    const body = stripComments(css.slice(open + 1, close)).trim();
    if (selector) rules.push({ selector, body });
    i = close + 1;
  }
  return rules;
}

/** All parsed CSS rules across every <style> block in app.html. */
function allRules() {
  return extractStyleBlocks(appHtml).flatMap(parseRules);
}

/** Individual selectors of a rule's selector list, trimmed. */
// Note: naive comma split is safe for this template's CSS (no commas inside
// :is()/:not()/attribute strings); revisit if that ever changes.
function selectorsOf(rule) {
  return rule.selector.split(',').map((s) => s.trim()).filter(Boolean);
}

/** Rules whose selector list contains exactly the given selector. */
function rulesFor(selector) {
  return allRules().filter((r) => selectorsOf(r).includes(selector));
}

// ─── Shared expectations about the declaration bodies ───────────────────────

const FULL_AXIS_CONTAIN = /overscroll-behavior\s*:/; // matches `overscroll-behavior:` but NOT `overscroll-behavior-x:`

describe('app.html momentum-scroll CSS (.table-responsive regression)', () => {
  it('has inline <style> blocks to parse', () => {
    expect(extractStyleBlocks(appHtml).length).toBeGreaterThan(0);
  });

  describe('.table-responsive', () => {
    const tableRules = rulesFor('.table-responsive');

    it('is declared with an exact selector rule', () => {
      expect(tableRules.length).toBeGreaterThan(0);
    });

    it('contains horizontal overscroll via overscroll-behavior-x: contain', () => {
      expect(
        tableRules.some((r) => r.body.includes('overscroll-behavior-x: contain')),
        'expected some .table-responsive rule to declare overscroll-behavior-x: contain'
      ).toBe(true);
    });

    it('never declares any full-axis overscroll-behavior property', () => {
      // If .table-responsive lands back in the vertical momentum group,
      // a vertical swipe over a wide table is swallowed again → page frozen.
      // The regex matches ANY full-axis value (contain/auto/…), not just contain.
      for (const r of tableRules) {
        expect(
          r.body,
          `rule "${r.selector}" must not use full-axis overscroll-behavior`
        ).not.toMatch(FULL_AXIS_CONTAIN);
      }
    });

    it('keeps -webkit-overflow-scrolling: touch (iOS fling momentum)', () => {
      expect(
        tableRules.some((r) => r.body.includes('-webkit-overflow-scrolling: touch'))
      ).toBe(true);
    });
  });

  describe('vertical momentum group', () => {
    const momentumRules = rulesFor('.sidebar-nav');

    it('still applies full-axis overscroll-behavior: contain to vertical scrollers', () => {
      expect(
        momentumRules.some((r) => r.body.includes('overscroll-behavior: contain')),
        'vertical scrollers should keep overscroll-behavior: contain'
      ).toBe(true);
    });

    it('does NOT include .table-responsive in the full-contain group', () => {
      for (const r of momentumRules) {
        expect(
          r.body.includes('overscroll-behavior: contain'),
          `rule "${r.selector}" should be the full-contain group`
        ).toBe(true);
        expect(
          selectorsOf(r),
          `rule "${r.selector}" must not swallow .table-responsive into full-axis contain`
        ).not.toContain('.table-responsive');
      }
    });
  });
});
