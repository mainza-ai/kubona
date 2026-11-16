#!/usr/bin/env node
// Merge the node and real-browser Vitest coverage maps, then enforce the
// hold-the-line floors. Browser component tests are real coverage; excluding
// them from the gate made the migration look like a regression.
import fs from 'node:fs';
import path from 'node:path';
import coverage from 'istanbul-lib-coverage';
const { createCoverageMap } = coverage;

const root = process.cwd();
const nodeFile = path.join(root, 'coverage-node', 'coverage-final.json');
const browserFile = path.join(root, 'coverage-browser', 'coverage-final.json');
for (const file of [nodeFile, browserFile]) {
  if (!fs.existsSync(file)) {
    console.error(`coverage: missing ${file}; run both coverage suites first`);
    process.exit(2);
  }
}
const map = createCoverageMap({});
map.merge(JSON.parse(fs.readFileSync(nodeFile, 'utf8')));
map.merge(JSON.parse(fs.readFileSync(browserFile, 'utf8')));
const summary = map.getCoverageSummary().data;
const floors = { statements: 53, branches: 54, functions: 51, lines: 56 };
const outDir = path.join(root, 'coverage');
fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(path.join(outDir, 'coverage-final.json'), JSON.stringify(map.toJSON()));
fs.writeFileSync(path.join(outDir, 'coverage-summary.json'), JSON.stringify({ total: summary }, null, 2) + '\n');
console.log('coverage summary (node + browser):');
for (const key of ['statements', 'branches', 'functions', 'lines']) {
  console.log(`  ${key}: ${summary[key].pct.toFixed(2)}% (floor ${floors[key]}%)`);
}
let failed = false;
for (const key of Object.keys(floors)) {
  if (summary[key].pct < floors[key]) {
    console.error(`ERROR: coverage for ${key} (${summary[key].pct.toFixed(2)}%) is below floor ${floors[key]}%`);
    failed = true;
  }
}
process.exit(failed ? 1 : 0);
