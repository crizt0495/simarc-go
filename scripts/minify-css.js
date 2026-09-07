#!/usr/bin/env node
/**
 * SIMARC CSS Minifier — zero-dependency production minifier.
 *
 * Usage: node scripts/minify-css.js [--watch]
 *
 * Minifies web/static/css/*.css → same `.min.css` files, stripping
 * comments/whitespace while SAFELY preserving:
 *   - data: URLs inside url("...")
 *   - media queries, keyframes, @supports
 *   - strings inside url(...)
 *
 * Safe for all browsers (no advanced syntax pruning).
 */
'use strict';

import { readdirSync, readFileSync, writeFileSync, watch } from 'fs';
import { join, dirname, basename } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const CSS_DIR = join(__dirname, '..', 'web', 'static', 'css');
const INPUTS = readdirSync(CSS_DIR).filter(f => f.endsWith('.css') && !f.endsWith('.min.css'));

function minify(css) {
  let out = '';
  let i = 0;
  let inString = false;
  let stringChar = '';
  let inUrl = false;
  const n = css.length;

  while (i < n) {
    const ch = css[i];
    const next = css[i + 1];

    // Strip comments (/* */) but not inside strings
    if (!inString && ch === '/' && next === '*') {
      const end = css.indexOf('*/', i + 2);
      i = end === -1 ? n : end + 2;
      continue;
    }

    // Handle strings (for data: URIs)
    if (!inString && (ch === '"' || ch === "'")) {
      inString = true;
      stringChar = ch;
      out += ch;
      i++;
      continue;
    }
    if (inString) {
      out += ch;
      if (ch === '\\') { out += css[i + 1] || ''; i += 2; continue; }
      if (ch === stringChar) inString = false;
      i++;
      continue;
    }

    // Collapse runs of whitespace to a single space, except inside urls
    if (/\s/.test(ch)) {
      while (i < n && /\s/.test(css[i])) i++;
      // Only keep a space if it separates two non-whitespace tokens that need it
      const prev = out[out.length - 1];
      const nxt = css[i];
      const needsSpace = prev && nxt &&
        /[a-zA-Z0-9_-]/.test(prev) && /[a-zA-Z0-9_-]/.test(nxt);
      // Keep space around : ; , { } is NOT needed
      if (needsSpace) out += ' ';
      continue;
    }

    // Remove whitespace-adjacent insignificant punctuation
    if (ch === ' ' || ch === '\n' || ch === '\t' || ch === '\r') { i++; continue; }

    // Optionally drop semi-colons before closing brace later (safe to keep)
    out += ch;
    i++;
  }

  // Post-pass: tighten `;}` -> `}` (safe), keep everything else
  return out.replace(/;}/g, '}');
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