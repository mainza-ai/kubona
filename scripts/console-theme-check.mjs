#!/usr/bin/env node
// D1 — verify the compiled theme, not just the source.
//
// The source lint proves nobody writes size-named utilities; this proves the
// reset actually removed Tailwind's default radius/text scales and that the
// role-named utilities used by the shell compile to the runtime aliases.
// Run after `next build` (the CI console job does).

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CHUNKS = path.join(ROOT, 'platform', 'console', '.next', 'static', 'chunks');

if (!fs.existsSync(CHUNKS)) {
  console.error('console-theme-check: no build output; run `next build` first');
  process.exit(1);
}

const cssFiles = fs
  .readdirSync(CHUNKS)
  .filter((name) => name.endsWith('.css'))
  .map((name) => path.join(CHUNKS, name))
  .filter((file) => fs.statSync(file).size > 10_000);
if (cssFiles.length === 0) {
  console.error('console-theme-check: no compiled CSS chunk over 10 KiB');
  process.exit(1);
}
const css = cssFiles.map((file) => fs.readFileSync(file, 'utf8')).join('\n');

const failures = [];
// Reset: the default scales must not exist at all.
for (const token of ['--radius-sm', '--radius-md', '--radius-lg', '--radius-xl', '--radius-2xl', '--radius-3xl']) {
  if (css.includes(`${token}:`)) failures.push(`${token} is still declared`);
}
for (const token of ['--text-xs', '--text-sm', '--text-base', '--text-lg', '--text-xl', '--text-2xl', '--text-3xl', '--text-4xl']) {
  if (css.includes(`${token}:`)) failures.push(`${token} is still declared`);
}
// Role names used by the shell must compile to the alias, not a literal.
// Tailwind may group a utility with an opacity variant (`.bg-chrome,.bg-chrome\/95{`),
// so accept a comma or brace after the selector.
for (const selector of ['.rounded-row', '.rounded-card', '.text-label', '.bg-surface', '.bg-chrome']) {
  const pattern = new RegExp(selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '[,{]');
  if (!pattern.test(css)) failures.push(`${selector} was not generated`);
}
// The legacy alias collided with the palette's overlay token.
if (/--color-card:\s*var\(--color-raised\)/.test(css)) {
  failures.push('--color-card still aliases --color-raised (legacy @theme collision)');
}

if (failures.length > 0) {
  console.error(`console-theme-check: ${failures.length} compiled-theme failure(s)`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(
  `console-theme-check: default radius/text scales absent; role utilities compile (${cssFiles.length} CSS chunk(s))`,
);
