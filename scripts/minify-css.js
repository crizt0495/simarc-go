#!/usr/bin/env node
/**
 * SIMARC CSS Minifier — zero-dependency production minifier.
 *
 * Usage: node scripts/minify-css.js [--watch] [--check]
 *
 *   (no flag)  rewrite every .min.css from its source
 *   --watch    rebuild on change
 *   --check    verify the committed .min.css matches its source; exit 1 if not
 *
 * Minifies web/static/css/*.css → same `.min.css` files, stripping
 * comments/whitespace while SAFELY preserving semantics:
 *   - data: URLs inside url("...")
 *   - media queries, keyframes, @supports
 *   - strings inside url(...)
 *
 * White-space safety rules:
 *   - A whitespace run is removed ONLY when the neighbouring token on at
 *     least one side is a "merge-safe" punctuation ({ } ; : , ! ( ). All
 *     other whitespace is preserved verbatim, because removing it can make
 *     a rule INVALID and silently dropped by browsers. Known corruption the
 *     old minifier caused (fixed here):
 *       • calc(var(--topbar-h) + 1.25rem)  →  calc(var(--topbar-h)+1.25rem)
 *         (CSS REQUIRES whitespace around + and - inside calc())
 *       • translateX(10px) rotate(45deg)  →  translateX(10px)rotate(45deg)
 *       • var(--x) 10px  →  var(--x)10px
 *       • 50% 50%  →  50%50%
 *       • .a *  →  .a*
 *   - calc()/min()/max()/clamp() bodies are additionally protected verbatim
 *     (belt-and-suspenders for arithmetic operators).
 */
'use strict';

import { readdirSync, readFileSync, writeFileSync, watch } from 'fs';
import { join, dirname, basename } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const CSS_DIR = join(__dirname, '..', 'web', 'static', 'css');
const INPUTS = readdirSync(CSS_DIR).filter(f => f.endsWith('.css') && !f.endsWith('.min.css'));

// Function names whose parenthesised body must be kept verbatim (arithmetic
// operators +/- inside them REQUIRE surrounding whitespace).
const PROTECT = new Set(['calc', 'min', 'max', 'clamp']);

// Punctuation after/before which a whitespace run may be safely removed.
const MERGE_SAFE = new Set(['{', '}', ';', ':', ',', '!', '(']);

/**
 * Replaces protected function calls (calc(...) etc.) with placeholder tokens
 * so the whitespace minifier never touches their bodies, then runs minify().
 */
function extractProtected(css) {
  const chunks = [];
  let i = 0;
  const n = css.length;
  let out = '';
  while (i < n) {
    const ch = css[i];
    if (ch === '"' || ch === "'") {
      const start = i;
      i++;
      while (i < n) {
        if (css[i] === '\\') { i += 2; continue; }
        if (css[i] === ch) { i++; break; }
        i++;
      }
      out += css.slice(start, i);
      continue;
    }
    let matched = false;
    for (const name of PROTECT) {
      if (css.startsWith(name + '(', i)) {
        const fnStart = i;
        i += name.length + 1; // past '('
        let depth = 1;
        while (i < n && depth > 0) {
          if (css[i] === '"' || css[i] === "'") {
            const q = css[i];
            i++;
            while (i < n) {
              if (css[i] === '\\') { i += 2; continue; }
              if (css[i] === q) { i++; break; }
              i++;
            }
            continue;
          }
          if (css[i] === '(') depth++;
          else if (css[i] === ')') depth--;
          i++;
        }
        let chunk = css.slice(fnStart, i);
        // Keep one trailing whitespace run so the following value token stays
        // separated from the closing paren (e.g. `calc(...) 1.25rem`).
        let trail = '';
        while (i < n && /\s/.test(css[i])) { trail += css[i]; i++; }
        chunk = chunk + trail;
        const ph = '\x00C' + chunks.length + '\x00';
        chunks.push(chunk);
        out += ph;
        matched = true;
        break;
      }
    }
    if (matched) continue;
    out += ch;
    i++;
  }
  return { css: out, chunks };
}

function restoreProtected(css, chunks) {
  for (let k = 0; k < chunks.length; k++) {
    css = css.split('\x00C' + k + '\x00').join(chunks[k]);
  }
  return css;
}

function minify(css) {
  let out = '';
  let i = 0;
  let inString = false;
  let stringChar = '';
  let braceDepth = 0;
  let inSelector = false;
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

    // Track selector context (before {)
    if (ch === '{') {
      inSelector = false;
      braceDepth++;
    } else if (ch === '}') {
      braceDepth = Math.max(0, braceDepth - 1);
    } else if (!inString && ch !== ' ' && ch !== '\n' && ch !== '\t' && ch !== '\r' && ch !== ';' && ch !== ':' && ch !== ',' && ch !== '{' && ch !== '}') {
      if (braceDepth === 0) inSelector = true;
    }

    // Whitespace run: remove ONLY when a neighbour is merge-safe punctuation,
    // otherwise keep it (space must separate value/simple-selector tokens).
    if (/\s/.test(ch)) {
      while (i < n && /\s/.test(css[i])) i++;
      const prev = out[out.length - 1];
      const nxt = css[i];
      const safePrev = prev !== undefined && MERGE_SAFE.has(prev);
      const safeNext = nxt !== undefined && MERGE_SAFE.has(nxt);
      const strip = (safePrev || safeNext) && prev !== undefined && nxt !== undefined;
      // A run at the very start or end of the file is irrelevant.
      const atEdge = prev === undefined || nxt === undefined;
      if (!atEdge && !strip) out += ' ';
      continue;
    }

    if (ch === ' ' || ch === '\n' || ch === '\t' || ch === '\r') { i++; continue; }

    out += ch;
    i++;
  }

  // Post-pass: tighten `;}` -> `}` (safe), keep everything else
  return out.replace(/;}/g, '}');
}

let changed = false;
const CHECK_ONLY = process.argv.includes('--check');

/**
 * Minifies every source file in CSS_DIR.
 *
 * In --check mode nothing is written: the freshly minified text is compared
 * against the committed `.min.css` so a stale artifact fails the build. The
 * pages load the minified file, not the source, so an out-of-date `.min.css`
 * silently ships the previous styles -- which is exactly how a topbar fix once
 * appeared to "do nothing".
 */
function build() {
  let stale = 0;

  for (const file of INPUTS) {
    const src = join(CSS_DIR, file);
    const dest = join(CSS_DIR, file.replace(/\.css$/, '.min.css'));
    const raw = readFileSync(src, 'utf8');
    const { css: protectedCss, chunks } = extractProtected(raw);
    const min = restoreProtected(minify(protectedCss), chunks);
    const kb = (raw.length / 1024).toFixed(1);
    const mkb = (min.length / 1024).toFixed(1);
    const pct = Math.max(0, Math.round((1 - min.length / raw.length) * 100));
    const name = basename(dest);

    if (CHECK_ONLY) {
      let current = null;
      try {
        current = readFileSync(dest, 'utf8');
      } catch {
        /* missing entirely */
      }
      if (current === min) {
        console.log(`✓ ${name} is up to date (${mkb}KB)`);
      } else {
        stale++;
        const why = current === null ? 'missing' : 'out of date';
        console.error(`✗ ${name} is ${why} — run: npm run css:minify`);
      }
      continue;
    }

    writeFileSync(dest, min);
    console.log(`✓ ${file}: ${kb}KB → ${mkb}KB  (-${pct}%)  → ${name}`);
  }

  if (CHECK_ONLY && stale > 0) {
    console.error(`\n${stale} minified CSS file(s) out of sync with their source.`);
    process.exit(1);
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