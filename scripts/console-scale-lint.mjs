#!/usr/bin/env node
// D1 — the Tailwind size-named scales are reset, so `rounded-xl` and `text-sm`
// compile to nothing. This guard fails on the source classes that would go
// silently square or inherit the wrong type size. Zero tolerance: the codemod
// to role-named radii/text is complete, so any hit is a regression.
//
//   node scripts/console-scale-lint.mjs

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SRC = path.join(ROOT, 'platform', 'console', 'src');
const DIRECTION = '(?:t|r|b|l|s|e|tl|tr|bl|br|ss|se|es|ee)';
const RADIUS = new RegExp(`(?<![\\w-])rounded(?:-${DIRECTION})?-(?:sm|md|lg|xl|2xl|3xl)(?![\\w-])`, 'g');
const TEXT = /(?<![\w-])text-(?:xs|sm|base|lg|xl|2xl|3xl|4xl)(?![\w-])/g;
const RADIUS_VAR = /--radius-(?:sm|md|lg|xl|2xl|3xl)\b/g;
const TEXT_VAR = /--text-(?:xs|sm|base|lg|xl|2xl|3xl|4xl)\b/g;

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
  const text = fs.readFileSync(file, 'utf8');
  const lines = text.split('\n');
  lines.forEach((line, index) => {
    for (const [label, pattern] of [
      ['size-named radius', RADIUS],
      ['size-named text', TEXT],
      ['reset radius variable', RADIUS_VAR],
      ['reset text variable', TEXT_VAR],
    ]) {
      pattern.lastIndex = 0;
      const match = pattern.exec(line);
      if (match) {
        failures.push(`${path.relative(ROOT, file)}:${index + 1}  ${label}  ${match[0]}`);
      }
    }
  });
}

if (failures.length > 0) {
  console.error(`console-scale-lint: ${failures.length} size-named scale use(s); use a role name`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log('console-scale-lint: no size-named radius/text utilities or reset variables');
