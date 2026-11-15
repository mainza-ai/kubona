#!/usr/bin/env node
// D6 — import-boundary enforcement. The console's layers only import
// downward; an upward import couples a leaf to a surface and is a build
// failure here, not a code-review note.
//
//   types  <  lib  <  domain  <  components  <  app
//
// Same-layer imports are legal. Test files are exempt (they may reach
// anywhere they exercise). No dependency is needed: the check is a source
// scan, deterministic and fast.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SRC = path.join(ROOT, 'platform', 'console', 'src');

const LAYERS = new Map([
  ['types', 0],
  ['lib', 1],
  ['domain', 2],
  ['components', 3],
  ['app', 4],
]);

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules' || entry.name === '.next') continue;
      walk(full, out);
    } else if (/\.(ts|tsx)$/.test(entry.name) && !/\.test\.(ts|tsx)$/.test(entry.name) && !entry.name.endsWith('.d.ts')) {
      out.push(full);
    }
  }
  return out;
}

function layerOfFile(file) {
  const rel = path.relative(SRC, file).split(path.sep);
  return LAYERS.has(rel[0]) ? { layer: rel[0], rank: LAYERS.get(rel[0]) } : null;
}

const IMPORT_RE = /(?:from\s+|import\s*\(\s*)['"]([^'"]+)['"]/g;

function resolveSpecifier(importer, spec) {
  if (spec.startsWith('@/')) return path.join(SRC, spec.slice(2));
  if (spec.startsWith('./') || spec.startsWith('../')) return path.resolve(path.dirname(importer), spec);
  return null;
}

const violations = [];
for (const file of walk(SRC)) {
  const importer = layerOfFile(file);
  if (!importer) continue;
  const lines = fs.readFileSync(file, 'utf8').split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    IMPORT_RE.lastIndex = 0;
    let match;
    while ((match = IMPORT_RE.exec(lines[i])) !== null) {
      const resolved = resolveSpecifier(file, match[1]);
      if (!resolved) continue;
      const target = layerOfFile(resolved);
      if (!target) continue;
      if (target.rank > importer.rank && target.layer !== importer.layer) {
        violations.push({
          file: path.relative(ROOT, file),
          line: i + 1,
          from: importer.layer,
          to: target.layer,
          spec: match[1],
        });
      }
    }
  }
}

if (violations.length > 0) {
  console.error(`console-boundaries: ${violations.length} upward import(s); layers are types < lib < domain < components < app`);
  for (const violation of violations) {
    console.error(`  ${violation.file}:${violation.line}  ${violation.from} → ${violation.to}  (${violation.spec})`);
  }
  process.exit(1);
}
console.log('console-boundaries: no upward imports (types < lib < domain < components < app)');
