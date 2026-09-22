#!/usr/bin/env node
/**
 * SIMARC WCAG Color-Contrast Audit — zero-dependency, CI-able.
 *
 * Usage: node scripts/wcag-audit.js
 * Exit code: 0 = all AA pass, 1 = failure.
 *
 * Parses the REAL palette tokens from enterprise.css :root (light) and
 * [data-theme="dark"], resolves var() chains, then verifies every
 * foreground/background pairing the design system relies on meets
 * WCAG 2.1 AA (4.5:1 body text, 3.0:1 large graphical/UI).
 */
'use strict';

import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(__dirname, '..', 'web', 'static', 'css', 'enterprise.css'), 'utf8');

/* ── helpers ─────────────────────────────────────────────────────── */
function hex2rgb(h) {
  h = ((h || '').replace('#', '')).trim();
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return null;
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}
function lin(c) { c /= 255; return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); }
function lum(hex) {
  const rgb = hex2rgb(hex);
  if (!rgb) throw new Error('bad hex: ' + hex);
  return 0.2126 * lin(rgb[0]) + 0.7152 * lin(rgb[1]) + 0.0722 * lin(rgb[2]);
}
function ratio(f, b) {
  const l1 = lum(f), l2 = lum(b);
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1];
  return (hi + 0.05) / (lo + 0.05);
}

/* Extract a token block `selector { … }` → { token: raw } */
function extract(selector) {
  const esc = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const m = css.match(new RegExp(esc + '\\s*\\{([\\s\\S]*?)\\n\\}'));
  if (!m) return {};
  const out = {};
  const lines = m[1].split('\n');
  for (const raw of lines) {
    const decl = raw.match(/^\s*(--[a-z0-9-]+)\s*:\s*(.+?)\s*;$/);
    if (decl) out[decl[1]] = decl[2].trim();
  }
  return out;
}

const L = extract(':root');
const D = extract('[data-theme="dark"]');

/* Resolve a token to a #RRGGBB string (follows var() chains, hex only). */
function resolve(raw, theme, depth = 0) {
  if (!raw) return null;
  let v = raw.trim();
  if (v.startsWith('var(')) {
    const name = v.match(/^var\(\s*(--[a-z0-9-]+)/);
    if (!name || depth > 6) return null;
    return resolve(theme[name], theme, depth + 1);
  }
  if (/^#[0-9a-fA-F]{6}$/.test(v)) return v.toUpperCase();
  return null;
}

/* Shorthand: token name → hex (throws if undefined or unresolved). */
function token(name, theme) {
  const hex = resolve(theme[name], theme);
  if (!hex) { console.error(`  ⚠  token tidak dapat di-resolve menjadi hex: ${name}`); process.exit(2); }
  return hex;
}

const checks = [];

/* ── LIGHT ────────────────────────────────────────────────────────── */
const LT = L, DT = D;
// body text
for (const ink of ['--nb-ink', '--nb-ink-2']) for (const bg of ['--nb-bg', '--nb-surface']) checks.push([`L ${ink.replace('--nb-','')} on ${bg.replace('--nb-','')}`, token(ink, LT), token(bg, LT), 4.5]);
for (const bg of ['--nb-bg', '--nb-surface', '--nb-surface-2', '--nb-surface-3']) checks.push(['L ink-3 on ' + bg.replace('--nb-',''), token('--nb-ink-3', LT), token(bg, LT), 4.5]);
for (const bg of ['--nb-bg', '--nb-bg-alt', '--nb-surface', '--nb-surface-2', '--nb-surface-3']) checks.push(['L ink-4 on ' + bg.replace('--nb-',''), token('--nb-ink-4', LT), token(bg, LT), 4.5]);
// on-accent (white) on solid fills
for (const fill of ['--nb-primary', '--nb-primary-dark', '--nb-primary-deep', '--nb-success', '--nb-success-dark', '--nb-danger', '--nb-danger-dark', '--nb-info', '--nb-info-dark', '--nb-warning', '--nb-warning-dark', '--nb-purple']) checks.push(['L on-accent on ' + fill.replace('--nb-',''), token('--nb-on-accent', LT), token(fill, LT), 4.5]);
// links & accents on surfaces
checks.push(['L primary on surface', token('--nb-primary', LT), token('--nb-surface', LT), 4.5]);
checks.push(['L primary-dark on surface', token('--nb-primary-dark', LT), token('--nb-surface', LT), 4.5]);
checks.push(['L primary on primary-light', token('--nb-primary', LT), token('--nb-primary-light', LT), 3.0]);
checks.push(['L primary-dark on primary-light', token('--nb-primary-dark', LT), token('--nb-primary-light', LT), 4.5]);
// soft-badge text on soft-badge bg (design system uses *-dark on *-light)
checks.push(['L success-dark on success-light', token('--nb-success-dark', LT), token('--nb-success-light', LT), 4.5]);
checks.push(['L danger-dark on danger-light', token('--nb-danger-dark', LT), token('--nb-danger-light', LT), 4.5]);
checks.push(['L danger on danger-light (icon/UI)', token('--nb-danger', LT), token('--nb-danger-light', LT), 3.0]);
checks.push(['L warning-dark on warning-light', token('--nb-warning-dark', LT), token('--nb-warning-light', LT), 4.5]);
checks.push(['L info-dark on info-light', token('--nb-info-dark', LT), token('--nb-info-light', LT), 4.5]);
checks.push(['L warning on surface', token('--nb-warning', LT), token('--nb-surface', LT), 4.5]);
checks.push(['L success on surface', token('--nb-success', LT), token('--nb-surface', LT), 4.5]);
checks.push(['L danger on surface', token('--nb-danger', LT), token('--nb-surface', LT), 4.5]);
checks.push(['L info on surface', token('--nb-info', LT), token('--nb-surface', LT), 4.5]);
checks.push(['L indigo on surface', token('--nb-indigo', LT), token('--nb-surface', LT), 3.0]);
checks.push(['L purple on surface', token('--nb-purple', LT), token('--nb-surface', LT), 3.0]);
checks.push(['L ink on surface-3', token('--nb-ink', LT), token('--nb-surface-3', LT), 4.5]);
checks.push(['L ink-2 on surface-3', token('--nb-ink-2', LT), token('--nb-surface-3', LT), 4.5]);

/* ── DARK ─────────────────────────────────────────────────────────── */
for (const ink of ['--nb-ink', '--nb-ink-2']) for (const bg of ['--nb-bg', '--nb-surface']) checks.push([`D ${ink.replace('--nb-','')} on ${bg.replace('--nb-','')}`, token(ink, DT), token(bg, DT), 4.5]);
for (const bg of ['--nb-bg', '--nb-surface', '--nb-surface-2', '--nb-surface-3']) checks.push(['D ink-3 on ' + bg.replace('--nb-',''), token('--nb-ink-3', DT), token(bg, DT), 4.5]);
for (const bg of ['--nb-bg', '--nb-surface', '--nb-surface-2', '--nb-surface-3']) checks.push(['D ink-4 on ' + bg.replace('--nb-',''), token('--nb-ink-4', DT), token(bg, DT), 4.5]);
// on-accent (dark ink) on lighter fills
for (const fill of ['--nb-primary', '--nb-primary-dark', '--nb-success', '--nb-danger', '--nb-info', '--nb-warning', '--nb-purple', '--nb-indigo']) checks.push(['D on-accent on ' + fill.replace('--nb-',''), token('--nb-on-accent', DT), token(fill, DT), 4.5]);
// accents on dark surfaces
for (const acc of ['--nb-primary', '--nb-success', '--nb-danger', '--nb-warning', '--nb-info']) checks.push(['D ' + acc.replace('--nb-','') + ' on surface', token(acc, DT), token('--nb-surface', DT), 4.5]);
checks.push(['D primary on surface-2', token('--nb-primary', DT), token('--nb-surface-2', DT), 4.5]);
checks.push(['D indigo on surface', token('--nb-indigo', DT), token('--nb-surface', DT), 3.0]);
checks.push(['D purple on surface', token('--nb-purple', DT), token('--nb-surface', DT), 3.0]);

/* ── run ──────────────────────────────────────────────────────────── */
let failures = 0;
console.log('═ SIMARC WCAG 2.1 AA — Color Contrast Audit (live enterprise palette) ═');
console.log('');
for (const [name, fg, bg, min] of checks) {
  const r = ratio(fg, bg);
  const ok = r >= min;
  if (!ok) failures++;
  console.log(`  ${ok ? 'PASS' : 'FAIL'}  ${name.padEnd(34)} ${r.toFixed(2).padStart(6)}  (AA ≥ ${min.toFixed(1)})`);
}

console.log('');
if (failures === 0) {
  console.log(`  ✅  ALL ${checks.length} pairings pass WCAG 2.1 AA`);
  process.exit(0);
} else {
  console.log(`  ❌  ${failures} failure(s) — fix before release`);
  process.exit(1);
}