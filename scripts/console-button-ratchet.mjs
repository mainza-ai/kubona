#!/usr/bin/env node
// Hold-the-line ratchet for raw <button> elements outside components/ui.
// Product code should use the shared Button/IconButton primitives; the
// existing migration debt is recorded and may only shrink.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SRC = path.join(ROOT, 'platform', 'console', 'src');
const UI = path.join(SRC, 'components', 'ui');
const BASELINE = path.join(ROOT, 'platform', 'contracts', 'console-button-baseline.json');

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
    if (file.startsWith(UI + path.sep)) continue;
    const text = fs.readFileSync(file, 'utf8');
    const count = (text.match(/<button\b/g) || []).length;
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
  console.log(`console-button-ratchet: wrote baseline (${current.total} raw buttons)`);
  process.exit(0);
}
if (!fs.existsSync(BASELINE)) {
  console.error('console-button-ratchet: baseline missing; run node scripts/console-button-ratchet.mjs --write');
  process.exit(2);
}
const baseline = JSON.parse(fs.readFileSync(BASELINE, 'utf8'));
if (current.total > baseline.total) {
  console.error(`console-button-ratchet: raw-button debt grew ${baseline.total} → ${current.total}`);
  for (const [file, count] of Object.entries(current.files)) {
    if (count > (baseline.files[file] ?? 0)) console.error(`  ${file}: ${count} (was ${baseline.files[file] ?? 0})`);
  }
  process.exit(1);
}
if (current.total < baseline.total) {
  console.log(`console-button-ratchet: debt fell ${baseline.total} → ${current.total}; update the baseline after review`);
}
console.log(`console-button-ratchet: ${current.total} known raw buttons (no growth)`);
