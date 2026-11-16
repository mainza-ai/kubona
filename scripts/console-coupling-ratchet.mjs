#!/usr/bin/env node
// Hold-the-line ratchet for components importing transport directly from
// lib/api. The domain layer may call lib/api; components should consume a
// domain projection. Existing debt is recorded and may only shrink.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const COMPONENTS = path.join(ROOT, 'platform', 'console', 'src', 'components');
const BASELINE = path.join(ROOT, 'platform', 'contracts', 'console-coupling-baseline.json');
const PATTERN = /import\s+(?!type\b)[^;]*?from\s+'@\/lib\/api';/gs;

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
  let imports = 0;
  for (const file of walk(COMPONENTS)) {
    const text = fs.readFileSync(file, 'utf8');
    const count = (text.match(PATTERN) || []).length;
    if (count) {
      files[path.relative(ROOT, file)] = count;
      imports += count;
    }
  }
  return { files: Object.keys(files).length, imports, entries: files };
}

const current = measure();
if (process.argv.includes('--write')) {
  fs.writeFileSync(BASELINE, `${JSON.stringify(current, null, 2)}\n`);
  console.log(`console-coupling-ratchet: wrote baseline (${current.files} files / ${current.imports} imports)`);
  process.exit(0);
}
if (!fs.existsSync(BASELINE)) {
  console.error('console-coupling-ratchet: baseline missing; run node scripts/console-coupling-ratchet.mjs --write');
  process.exit(2);
}
const baseline = JSON.parse(fs.readFileSync(BASELINE, 'utf8'));
if (current.files > baseline.files || current.imports > baseline.imports) {
  console.error(`console-coupling-ratchet: coupling grew ${baseline.files}/${baseline.imports} → ${current.files}/${current.imports}`);
  process.exit(1);
}
if (current.files < baseline.files || current.imports < baseline.imports) {
  console.log(`console-coupling-ratchet: coupling fell ${baseline.files}/${baseline.imports} → ${current.files}/${current.imports}; update the baseline after review`);
}
console.log(`console-coupling-ratchet: ${current.files} component files / ${current.imports} direct API imports (no growth)`);
