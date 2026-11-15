#!/usr/bin/env node
// D1 — semantic colour utilities are the contract. Every `--color-*` token
// used by a bg/text/border/ring/gradient/fill utility is mapped through
// @theme, so the arbitrary form is no longer needed. Zero tolerance: the
// codemod left none, and a new one would bypass the theme aliases.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SRC = path.join(ROOT, 'platform', 'console', 'src');
const PATTERN =
  /(?<![\w-])(?:bg|text|border|ring|from|to|via|fill|stroke|decoration|outline|divide|accent|caret)-\[var\(--color-[a-z0-9-]+\)\]/g;

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules' || entry.name === '.next') continue;
      walk(full, out);
    } else if (/\.(ts|tsx|css)$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

const failures = [];
for (const file of walk(SRC)) {
  const lines = fs.readFileSync(file, 'utf8').split('\n');
  lines.forEach((line, index) => {
    PATTERN.lastIndex = 0;
    const match = PATTERN.exec(line);
    if (match) failures.push(`${path.relative(ROOT, file)}:${index + 1}  ${match[0]}`);
  });
}

if (failures.length > 0) {
  console.error(`console-color-utility-lint: ${failures.length} arbitrary colour utilit(ies); use the semantic token`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log('console-color-utility-lint: no arbitrary colour-var utilities');
