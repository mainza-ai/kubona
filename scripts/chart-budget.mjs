#!/usr/bin/env node
// Measure the chart implementation itself (chart.ts + DataChart.tsx), not the
// React runtime or the page chunk. Deterministic, offline, and fast: the
// source is transpiled by TypeScript and minified by the terser build already
// shipped with Next. Budgets are for the implementation, not a browser chunk.
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CONSOLE = path.join(ROOT, 'platform', 'console');
const require = createRequire(path.join(CONSOLE, 'package.json'));
const ts = require('typescript');
const terser = require('next/dist/compiled/terser');

const MIN_BUDGET = 60 * 1024;
const GZIP_BUDGET = 20 * 1024;
const FILES = ['src/components/data/chart.ts', 'src/components/data/DataChart.tsx'];

let source = '';
for (const file of FILES) {
  const full = path.join(CONSOLE, file);
  source += ts.transpileModule(fs.readFileSync(full, 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText + '\n';
}

const minified = await terser.minify(source);
if (!minified.code) throw new Error('chart-budget: minification produced no code');
const bytes = Buffer.from(minified.code);
const gzip = zlib.gzipSync(bytes);
const status = bytes.length <= MIN_BUDGET && gzip.length <= GZIP_BUDGET ? 'OK' : 'OVER BUDGET';
console.log(
  `chart-budget: ${(bytes.length / 1024).toFixed(1)} KB min / ${(gzip.length / 1024).toFixed(1)} KB gzip ` +
  `(budget ${MIN_BUDGET / 1024} KB / ${GZIP_BUDGET / 1024} KB) · ${status}`,
);
if (status !== 'OK') process.exit(1);
