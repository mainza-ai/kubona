#!/usr/bin/env node
// Hold-the-line ratchet for raw palette/legacy utilities in console product
// code. Existing debt is recorded; new debt fails CI. Run --write after a
// deliberate migration lowers the baseline.
//
// D4: `transition: all` is banned outright (zero tolerance, no baseline): it
// animates layout and paint the author never intended, and it defeats the
// explicit motion vocabulary in styles/tokens.css.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SRC = path.join(ROOT, 'platform', 'console', 'src');
const BASELINE = path.join(ROOT, 'platform', 'contracts', 'console-token-baseline.json');
const PATTERN = /text-slate-|bg-cyan-|rounded-md\b|text-\[(?:8|9|10)px\]/g;

function walk(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else if (entry.isFile() && /\.(tsx|ts)$/.test(entry.name) && !/\.test\.(tsx|ts)$/.test(entry.name)) out.push(full);
  }
  return out;
}

function measure() {
  const files = {};
  let total = 0;
  for (const file of walk(SRC)) {
    const text = fs.readFileSync(file, 'utf8');
    const count = (text.match(PATTERN) || []).length;
    if (count) {
      files[path.relative(ROOT, file)] = count;
      total += count;
    }
  }
  return { total, files };
}

const CSS_ROOTS = ['app', 'styles'].map((dir) => path.join(SRC, dir));
const CSS_MOTION_PATTERN = /(?:transition|animation)\s*:\s*all\b/g;
// Tailwind's `transition-all` compiles to transition-property: all — the same
// layout/paint surprise the CSS rule bans. Scan source classes too.
const SOURCE_MOTION_PATTERN = /(?<![\w-])transition-all(?![\w-])/g;

function cssFiles() {
  const out = [];
  for (const root of CSS_ROOTS) {
    if (!fs.existsSync(root)) continue;
    for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
      const full = path.join(root, entry.name);
      if (entry.isFile() && entry.name.endsWith('.css')) out.push(full);
    }
  }
  return out;
}

function measureCssMotion() {
  const files = {};
  let total = 0;
  for (const file of cssFiles()) {
    const css = fs.readFileSync(file, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
    const count = (css.match(CSS_MOTION_PATTERN) || []).length;
    if (count) {
      files[path.relative(ROOT, file)] = count;
      total += count;
    }
  }
  for (const file of walk(SRC)) {
    const source = fs.readFileSync(file, 'utf8');
    const count = (source.match(SOURCE_MOTION_PATTERN) || []).length;
    if (count) {
      files[path.relative(ROOT, file)] = count;
      total += count;
    }
  }
  return { total, files };
}

const current = measure();
if (process.argv.includes('--write')) {
  fs.writeFileSync(BASELINE, `${JSON.stringify(current, null, 2)}\n`);
  console.log(`console-token-lint: wrote baseline (${current.total} occurrences)`);
  process.exit(0);
}
if (!fs.existsSync(BASELINE)) {
  console.error('console-token-lint: baseline missing; run node scripts/console-token-lint.mjs --write');
  process.exit(2);
}
const cssMotion = measureCssMotion();
if (cssMotion.total > 0) {
  console.error(`console-token-lint: transition/animation "all" is banned (${cssMotion.total} occurrence(s))`);
  for (const [file, count] of Object.entries(cssMotion.files)) console.error(`  ${file}: ${count}`);
  process.exit(1);
}
const baseline = JSON.parse(fs.readFileSync(BASELINE, 'utf8'));
if (current.total > baseline.total) {
  console.error(`console-token-lint: raw palette debt grew ${baseline.total} → ${current.total}`);
  for (const [file, count] of Object.entries(current.files)) {
    if (count > (baseline.files[file] ?? 0)) console.error(`  ${file}: ${count} (was ${baseline.files[file] ?? 0})`);
  }
  process.exit(1);
}
if (current.total < baseline.total) {
  console.log(`console-token-lint: debt fell ${baseline.total} → ${current.total}; update the baseline after review`);
}
console.log(`console-token-lint: ${current.total} known raw-palette occurrences (no growth)`);
