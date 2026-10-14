#!/usr/bin/env node
/**
 * SIMARC CSS Minifier — zero-dependency production minifier.
 *
 * Usage: node tooling/minify-css.js [--watch]
 *
 * Minifies web/static/css/*.css → same `.min.css` files, stripping
 * comments/indentation while SAFELY preserving anything where whitespace
 * is significant:
 *   - descendant selectors        `.a .b`   (NOT `.a.b`)
 *   - calc() arithmetic           `calc(100% - 1rem)` (spaces REQUIRED)
 *   - media/supports preludes     `@media (min-width: 992px)`
 *   - strings / data: URLs inside url(...)
 *
 * Only whitespace adjacent to `{ } ; , (` is removed, because that is
 * always safe in CSS. Everything else keeps at most a single space.
 * Safe for all browsers (no advanced syntax pruning).
 */
'use strict';

import { readdirSync, readFileSync, writeFileSync, watch } from 'fs';
import { join, dirname, basename } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const CSS_DIR = join(__dirname, '..', 'web', 'static', 'css');
const INPUTS = readdirSync(CSS_DIR).filter(f => f.endsWith('.css') && !f.endsWith('.min.css'));

// Whitespace immediately after one of these is never significant.
const NO_SPACE_AFTER = new Set(['{', '}', ';', ',', '(']);
// Whitespace immediately before one of these is never significant.
const NO_SPACE_BEFORE = new Set(['{', '}', ';', ',', ')']);

function needSpace(prev, next) {
  if (!prev) return false;
  if (NO_SPACE_AFTER.has(prev)) return false;
  if (NO_SPACE_BEFORE.has(next)) return false;
  return true;
}

function minify(css) {
  let out = '';
  let i = 0;
  const n = css.length;
  let inString = false;
  let stringChar = '';
  let pendingWs = false;

  const flushWs = (next) => {
    if (pendingWs && needSpace(out[out.length - 1], next)) out += ' ';
    pendingWs = false;
  };

  while (i < n) {
    const ch = css[i];

    // Strip comments (/* */) — but never inside a string.
    if (!inString && ch === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2);
      i = end === -1 ? n : end + 2;
      // A comment behaves like the whitespace it replaces.
      pendingWs = true;
      continue;
    }

    // Inside a string: copy verbatim (handles escaped chars & data: URIs).
    if (inString) {
      out += ch;
      if (ch === '\\') { out += css[i + 1] || ''; i += 2; continue; }
      if (ch === stringChar) inString = false;
      i++;
      continue;
    }

    // Start of a string.
    if (ch === '"' || ch === "'") {
      flushWs(ch);
      inString = true;
      stringChar = ch;
      out += ch;
      i++;
      continue;
    }

    // Whitespace run → remember it, decide later.
    if (/\s/.test(ch)) { pendingWs = true; i++; continue; }

    // A real token character.
    flushWs(ch);

    // Drop a `;` that sits right before `}` (always safe).
    if (ch === '}' && out[out.length - 1] === ';') out = out.slice(0, -1);

    out += ch;
    i++;
  }

  return out;
}

let changed = false;

function build() {
  for (const file of INPUTS) {
    const src = join(CSS_DIR, file);
    const dest = join(CSS_DIR, file.replace(/\.css$/, '.min.css'));
    const raw = readFileSync(src, 'utf8');
    const min = minify(raw);
    writeFileSync(dest, min);

    const kb = (raw.length / 1024).toFixed(1);
    const mkb = (min.length / 1024).toFixed(1);
    const pct = Math.max(0, Math.round((1 - min.length / raw.length) * 100));
    console.log(`✓ ${file}: ${kb}KB → ${mkb}KB  (-${pct}%)  → ${basename(dest)}`);
  }
  changed = true;
}

build();

if (process.argv.includes('--watch')) {
  console.log('\nWatching for changes... Ctrl+C to stop.');
  watch(CSS_DIR, (_ev, filename) => {
    if (filename && filename.endsWith('.css') && !filename.endsWith('.min.css')) {
      console.log(`\n[change] ${filename} — rebuilding...`);
      build();
    }
  });
}

process.exit(changed ? 0 : 0);
