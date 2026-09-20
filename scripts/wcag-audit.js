#!/usr/bin/env node
/**
 * SIMARC WCAG Color-Contrast Audit — zero-dependency, CI-able.
 *
 * Usage: node scripts/wcag-audit.js
 * Exit code: 0 = all AA pass, 1 = failure.
 *
 * Parses the REAL palette tokens from neo-brutalism.css :root (light) and
 * [data-theme="dark"], then verifies every foreground/background pairing
 * the design system actually relies on meets WCAG 2.1 AA
 * (4.5:1 body text, 3.0:1 large graphical/UI).
 */
'use strict';

import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(__dirname, '..', 'web', 'static', 'css', 'neo-brutalism.css'), 'utf8');

function hex2rgb(h, fallback = [0, 0, 0]) {
  h = (h || '').replace('#', '');
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return fallback;
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

function lin(c) { c /= 255; return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); }

function lum(hex) {
  const [r, g, b] = hex2rgb(hex);
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function ratio(f, b) {
  const l1 = lum(f), l2 = lum(b);
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1];
  return (hi + 0.05) / (lo + 0.05);
}

// ── Extract a token block's declarations from the CSS ──
function extract(selector) {
  const esc = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const m = css.match(new RegExp(esc + '\\s*\\{([^}]*)\\}'));
  if (!m) return {};
  const out = {};
  for (const decl of m[1].matchAll(/(--[a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;/g)) {
    out[decl[1]] = decl[2].toUpperCase();
  }
  return out;
}

const L = extract(':root');
const D = extract('[data-theme="dark"]');

// Resolve a CSS variable (strip var(--x)) — tokens only contain hex by design.
function v(ref, theme) {
  const name = (ref || '').replace(/^var\((--[a-z0-9-]+)\).*$/, '$1');
  const val = theme[name];
  if (!val) { console.error('  ⚠  undefined token: ' + name); process.exit(2); }
  return val;
}

const checks = [
  // ── Light mode ──
  ['L ink on bg',              v('--nb-ink', L),        v('--nb-bg', L),        4.5],
  ['L ink on surface',         v('--nb-ink', L),        v('--nb-surface', L),   4.5],
  ['L ink-2 on bg',            v('--nb-ink-2', L),      v('--nb-bg', L),        4.5],
  ['L ink-2 on surface',       v('--nb-ink-2', L),      v('--nb-surface', L),   4.5],
  ['L ink-3 on bg',            v('--nb-ink-3', L),      v('--nb-bg', L),        4.5],
  ['L ink-3 on surface',       v('--nb-ink-3', L),      v('--nb-surface', L),   4.5],
  ['L ink-3 on surface-2',     v('--nb-ink-3', L),      v('--nb-surface-2', L), 4.5],
  ['L ink-3 on surface-3',     v('--nb-ink-3', L),      v('--nb-surface-3', L), 4.5],
  ['L ink-4 on bg',            v('--nb-ink-4', L),      v('--nb-bg', L),        4.5],
  ['L ink-4 on bg-alt',        v('--nb-ink-4', L),      v('--nb-bg-alt', L),    4.5],
  ['L ink-4 on surface',       v('--nb-ink-4', L),      v('--nb-surface', L),   4.5],
  ['L ink-4 on surface-2',     v('--nb-ink-4', L),      v('--nb-surface-2', L), 4.5],
  ['L ink-4 on surface-3',     v('--nb-ink-4', L),      v('--nb-surface-3', L), 4.5],
  // Light buttons: ink/white on fills
  ['L on-primary on #FF4D00',  v('--nb-on-primary', L), v('--nb-primary', L),   4.5],
  ['L on-primary-hover on dark', v('--nb-on-primary-hover', L), v('--nb-primary-dark', L), 4.5],
  ['L on-success on success',  v('--nb-on-success', L), v('--nb-success', L),   4.5],
  ['L on-danger on danger',    v('--nb-on-danger', L),  v('--nb-danger', L),    4.5],
  ['L on-warning on warning',  v('--nb-on-warning', L), v('--nb-warning', L),   4.5],
  ['L on-info on info',        v('--nb-on-info', L),    v('--nb-info', L),      4.5],
  ['L on-dark on ink',         v('--nb-on-dark', L),    v('--nb-ink', L),       4.5],
  ['L on-orange on orange',    v('--nb-on-primary', L), v('--nb-orange', L),    4.5],
  // Light: text/link accents on surfaces (4.5) & UI (3.0)
  ['L primary-dark on surface', v('--nb-primary-dark', L), v('--nb-surface', L), 4.5],
  ['L primary-dark on bg',     v('--nb-primary-dark', L),  v('--nb-bg', L),       4.5],
  ['L primary-dark on primary-light', v('--nb-primary-dark', L), v('--nb-primary-light', L), 4.5],
  ['L success on surface',     v('--nb-success', L),    v('--nb-surface', L),   4.5],
  ['L success on success-light', v('--nb-success', L),  v('--nb-success-light', L), 4.5],
  ['L danger on surface',      v('--nb-danger', L),     v('--nb-surface', L),   4.5],
  ['L danger-dark on danger-light', v('--nb-danger-dark', L), v('--nb-danger-light', L), 4.5],
  ['L info on surface',        v('--nb-info', L),       v('--nb-surface', L),   4.5],
  ['L info on info-light',     v('--nb-info', L),       v('--nb-info-light', L), 4.5],
  ['L warning-dark on surface', v('--nb-warning-dark', L), v('--nb-surface', L), 4.5],
  ['L warning-dark on warning-light', v('--nb-warning-dark', L), v('--nb-warning-light', L), 4.5],
  ['L indigo on surface',      v('--nb-indigo', L),     v('--nb-surface', L),   4.5],
  ['L purple on surface',      v('--nb-purple', L),     v('--nb-surface', L),   3.0],
  ['L ink-2 on status-inactive-ish', v('--nb-ink-4', L), v('--nb-surface-3', L), 4.5],

  // ── Dark mode ──
  ['D on-primary on primary',  v('--nb-on-primary', D), v('--nb-primary', D),   4.5],
  ['D on-primary-hover on dark', v('--nb-on-primary-hover', D), v('--nb-primary-dark', D), 4.5],
  ['D on-success on success',  v('--nb-on-success', D), v('--nb-success', D),   4.5],
  ['D on-danger on danger',    v('--nb-on-danger', D),  v('--nb-danger', D),    4.5],
  ['D on-warning on warning',  v('--nb-on-warning', D), v('--nb-warning', D),   4.5],
  ['D on-info on info',        v('--nb-on-info', D),    v('--nb-info', D),      4.5],
  ['D on-dark on ink',         v('--nb-on-dark', D),    v('--nb-ink', D),       4.5],
  ['D on-orange on orange',    v('--nb-on-primary', D), v('--nb-orange', D),    4.5],
  // Dark body text
  ['D ink on bg',              v('--nb-ink', D),        v('--nb-bg', D),        4.5],
  ['D ink on surface',         v('--nb-ink', D),        v('--nb-surface', D),   4.5],
  ['D ink-3 on surface',       v('--nb-ink-3', D),      v('--nb-surface', D),   4.5],
  ['D ink-4 on bg',            v('--nb-ink-4', D),      v('--nb-bg', D),        4.5],
  ['D ink-4 on surface',       v('--nb-ink-4', D),      v('--nb-surface', D),   4.5],
  ['D ink-4 on surface-2',     v('--nb-ink-4', D),      v('--nb-surface-2', D), 4.5],
  ['D ink-4 on surface-3',     v('--nb-ink-4', D),      v('--nb-surface-3', D), 4.5],
  // Dark accents on surfaces (3.0 UI / 4.5 links)
  ['D primary-dark on surface', v('--nb-primary-dark', D), v('--nb-surface', D), 4.5],
  ['D primary on surface',     v('--nb-primary', D),    v('--nb-surface', D),   4.5],
  ['D primary on surface-2',   v('--nb-primary', D),    v('--nb-surface-2', D), 4.5],
  ['D success on surface',     v('--nb-success', D),    v('--nb-surface', D),   4.5],
  ['D danger on surface',      v('--nb-danger', D),     v('--nb-surface', D),   4.5],
  ['D info on surface',        v('--nb-info', D),       v('--nb-surface', D),   4.5],
  ['D warning on surface',     v('--nb-warning', D),    v('--nb-surface', D),   4.5],
  ['D purple on surface',      v('--nb-purple', D),     v('--nb-surface', D),   4.5],
  ['D indigo on surface',      v('--nb-indigo', D),     v('--nb-surface', D),   4.5],
];

let failures = 0;
console.log('═ SIMARC WCAG 2.1 AA — Color Contrast Audit (live palette) ═');
console.log('');

for (const [name, fg, bg, min] of checks) {
  const r = ratio(fg, bg);
  const ok = r >= min;
  if (!ok) failures++;
  console.log(`  ${ok ? 'PASS' : 'FAIL'}  ${name.padEnd(28)} ${r.toFixed(2).padStart(6)}  (AA ≥ ${min.toFixed(1)})`);
}

console.log('');
if (failures === 0) {
  console.log(`  ✅  ALL ${checks.length} pairings pass WCAG 2.1 AA`);
  process.exit(0);
} else {
  console.log(`  ❌  ${failures} failure(s) — fix before release`);
  process.exit(1);
}