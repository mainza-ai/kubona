#!/usr/bin/env node
// Hold-the-line ratchet for raw palette/legacy utilities in console product
// code. Existing debt is recorded; new debt fails CI. Run --write after a
// deliberate migration lowers the baseline.
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
