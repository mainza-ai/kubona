#!/usr/bin/env node
// Hold-the-line ratchet for raw <input>/<select>/<textarea> outside
// components/ui. Product forms should use Field/Select/Toggle primitives;
// existing migration debt is recorded and may only shrink.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const COMPONENTS = path.join(ROOT, 'platform', 'console', 'src', 'components');
const UI = path.join(COMPONENTS, 'ui');
const BASELINE = path.join(ROOT, 'platform', 'contracts', 'console-form-baseline.json');
const PATTERN = /<(?:input|select|textarea)\b/g;

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
  let controls = 0;
  for (const file of walk(COMPONENTS)) {
    if (file.startsWith(UI + path.sep)) continue;
    const text = fs.readFileSync(file, 'utf8');
    const count = (text.match(PATTERN) || []).length;
    if (count) {
      files[path.relative(ROOT, file)] = count;
      controls += count;
    }
  }
  return { files: Object.keys(files).length, controls, entries: files };
}

const current = measure();
if (process.argv.includes('--write')) {
  fs.writeFileSync(BASELINE, `${JSON.stringify(current, null, 2)}\n`);
  console.log(`console-form-ratchet: wrote baseline (${current.files} files / ${current.controls} controls)`);
  process.exit(0);
}
if (!fs.existsSync(BASELINE)) {
  console.error('console-form-ratchet: baseline missing; run node scripts/console-form-ratchet.mjs --write');
  process.exit(2);
}
const baseline = JSON.parse(fs.readFileSync(BASELINE, 'utf8'));
if (current.files > baseline.files || current.controls > baseline.controls) {
  console.error(`console-form-ratchet: raw form debt grew ${baseline.files}/${baseline.controls} → ${current.files}/${current.controls}`);
  process.exit(1);
}
if (current.files < baseline.files || current.controls < baseline.controls) {
  console.log(`console-form-ratchet: debt fell ${baseline.files}/${baseline.controls} → ${current.files}/${current.controls}; update the baseline after review`);
}
console.log(`console-form-ratchet: ${current.files} component files / ${current.controls} raw controls (no growth)`);
