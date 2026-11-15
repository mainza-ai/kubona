#!/usr/bin/env node
// D8 — freeze new react-test-renderer component tests.
//
// react-test-renderer cannot observe focus, keyboard, layout, or CSS, so it is
// a dead end for the component suite. Existing files are grandfathered in
// platform/contracts/console-test-renderer-baseline.json; a new file that
// imports it fails, and a stale baseline entry fails too so the list shrinks
// as files migrate. New interactive coverage belongs in
// `*.component.test.tsx` (vitest.browser.config.ts).
//
//   node scripts/console-test-renderer-ratchet.mjs
//   node scripts/console-test-renderer-ratchet.mjs --write   # after migration

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CONSOLE = path.join(ROOT, 'platform', 'console');
const SRC = path.join(CONSOLE, 'src');
const BASELINE = path.join(ROOT, 'platform', 'contracts', 'console-test-renderer-baseline.json');
const IMPORT_RE = /(?:from\s+['"]react-test-renderer['"]|require\(\s*['"]react-test-renderer['"]\s*\))/;

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules' || entry.name === '.next') continue;
      walk(full, out);
    } else if (entry.name.endsWith('.test.tsx') || entry.name.endsWith('.test.ts')) {
      out.push(full);
    }
  }
  return out;
}

const current = walk(SRC)
  .filter((file) => IMPORT_RE.test(fs.readFileSync(file, 'utf8')))
  .map((file) => path.relative(CONSOLE, file).split(path.sep).join('/'))
  .sort();

if (process.argv.includes('--write')) {
  fs.writeFileSync(
    BASELINE,
    `${JSON.stringify({
      version: 1,
      note: 'D8 ratchet: files allowed to import react-test-renderer. Delete entries as files migrate to *.component.test.tsx browser tests. Do not add entries.',
      files: current,
    }, null, 2)}\n`,
  );
  console.log(`console-test-renderer-ratchet: wrote ${current.length} grandfathered file(s)`);
  process.exit(0);
}

if (!fs.existsSync(BASELINE)) {
  console.error(`console-test-renderer-ratchet: missing ${path.relative(ROOT, BASELINE)}; run with --write once`);
  process.exit(1);
}

const baseline = JSON.parse(fs.readFileSync(BASELINE, 'utf8'));
const allowed = new Set(baseline.files ?? []);
const seen = new Set(current);
const added = current.filter((file) => !allowed.has(file));
const stale = [...allowed].filter((file) => !seen.has(file)).sort();

if (added.length > 0) {
  console.error(`console-test-renderer-ratchet: ${added.length} new react-test-renderer test(s)`);
  for (const file of added) console.error(`  ${file}`);
  console.error('  new interactive coverage belongs in a *.component.test.tsx browser test');
}
if (stale.length > 0) {
  console.error(`console-test-renderer-ratchet: ${stale.length} stale baseline entr(ies); delete them`);
  for (const file of stale) console.error(`  ${file}`);
}
if (added.length > 0 || stale.length > 0) process.exit(1);

console.log(`console-test-renderer-ratchet: ${current.length} grandfathered file(s), no growth`);
