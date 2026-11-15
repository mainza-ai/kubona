#!/usr/bin/env node
// D7 — per-chunk bundle budgets from the real Next build, not an estimate.
//
// Reads the app-router client reference manifest and build manifest, sums the
// FIRST-LOAD chunks of a route, and enforces:
//   - raw + gzip first-load ceilings per route (with the explicit allowance
//     printed as headroom, never silently consumed),
//   - forbidden libraries in first-load (they must stay lazy),
//   - large lazy chunks must be recorded in `recorded_lazy_chunks` by a
//     stable content marker plus a reason; an unrecorded one fails, and a
//     stale record fails too.
//
// Run after `next build`; the CI console job does exactly that.

import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CONSOLE = path.join(ROOT, 'platform', 'console');
const NEXT = path.join(CONSOLE, '.next');
const CONTRACT = path.join(ROOT, 'platform', 'contracts', 'console-bundle-budgets.json');

function fail(message) {
  console.error(`console-bundle-budgets: ${message}`);
  process.exit(1);
}

if (!fs.existsSync(CONTRACT)) fail(`missing ${path.relative(ROOT, CONTRACT)}`);
if (!fs.existsSync(path.join(NEXT, 'build-manifest.json'))) {
  fail('no build output; run `next build` first (this gate runs after pnpm build in CI)');
}

const contract = JSON.parse(fs.readFileSync(CONTRACT, 'utf8'));

function readClientManifest(route) {
  const file = path.join(NEXT, 'server', 'app', route === '/' ? 'page' : route.replace(/^\//, ''), 'page_client-reference-manifest.js');
  const fallback = route === '/' ? path.join(NEXT, 'server', 'app', 'page_client-reference-manifest.js') : file;
  const target = fs.existsSync(file) ? file : fallback;
  if (!fs.existsSync(target)) return null;
  const text = fs.readFileSync(target, 'utf8');
  // The file is `globalThis.__RSC_MANIFEST = ... || {};` followed by the
  // route assignment `globalThis.__RSC_MANIFEST["/page"] = {...};`. Anchor on
  // the assignment, not the first `{` (which belongs to the `|| {}`).
  const assignment = text.indexOf('__RSC_MANIFEST[');
  const equals = assignment < 0 ? -1 : text.indexOf('= {', assignment);
  const start = equals < 0 ? -1 : equals + 2;
  const end = text.lastIndexOf('}');
  if (start < 0 || end <= start) return null;
  try {
    return JSON.parse(text.slice(start, end + 1));
  } catch {
    return null;
  }
}

function canonicalChunk(chunkPath) {
  const cleaned = chunkPath.replace(/^\/_next\//, '').replace(/^\//, '');
  return cleaned;
}

function chunkFile(chunkPath) {
  return path.join(NEXT, canonicalChunk(chunkPath));
}

function sizeOf(chunkPath) {
  const file = chunkFile(chunkPath);
  if (!fs.existsSync(file)) return null;
  const raw = fs.statSync(file).size;
  const gzip = zlib.gzipSync(fs.readFileSync(file)).length;
  return { raw, gzip };
}

const buildManifest = JSON.parse(fs.readFileSync(path.join(NEXT, 'build-manifest.json'), 'utf8'));
const sharedChunks = [...(buildManifest.rootMainFiles ?? []), ...(buildManifest.polyfillFiles ?? [])].map(canonicalChunk);

let failed = false;
for (const [route, budget] of Object.entries(contract.routes ?? {})) {
  const manifest = readClientManifest(route);
  if (!manifest) {
    fail(`no client manifest for route ${route}`);
  }
  const modules = Object.values(manifest.clientModules ?? {});
  const firstLoad = new Set(sharedChunks);
  for (const mod of modules) {
    for (const chunk of mod.chunks ?? []) {
      if (mod.async !== true) firstLoad.add(canonicalChunk(chunk));
    }
  }
  let raw = 0;
  let gzip = 0;
  let largest = { chunk: '', raw: 0 };
  const contents = [];
  for (const chunk of firstLoad) {
    const size = sizeOf(chunk);
    if (!size) continue;
    raw += size.raw;
    gzip += size.gzip;
    if (size.raw > largest.raw) largest = { chunk, raw: size.raw };
    contents.push({ chunk, text: fs.readFileSync(chunkFile(chunk), 'utf8') });
  }
  const rawKb = raw / 1024;
  const gzipKb = gzip / 1024;
  const allowanceRaw = budget.allowance_raw_kb ?? 0;
  const allowanceGzip = budget.allowance_gzip_kb ?? 0;
  const rawCeiling = budget.recorded_raw_kb + allowanceRaw;
  const gzipCeiling = budget.recorded_gzip_kb + allowanceGzip;
  for (const [label, value, ceiling] of [
    ['recorded raw', budget.recorded_raw_kb, rawCeiling],
    ['allowance raw', allowanceRaw, rawCeiling],
    ['recorded gzip', budget.recorded_gzip_kb, gzipCeiling],
    ['allowance gzip', allowanceGzip, gzipCeiling],
  ]) {
    if (!Number.isFinite(value) || value < 0 || !Number.isFinite(ceiling)) {
      fail(`route ${route} has an invalid ${label} value`);
    }
  }
  console.log(
    `console-bundle-budgets: ${route} first-load ${rawKb.toFixed(1)} KiB raw / ${gzipKb.toFixed(1)} KiB gzip ` +
      `(recorded ${budget.recorded_raw_kb} / ${budget.recorded_gzip_kb} KiB + allowance ` +
      `${allowanceRaw} / ${allowanceGzip} KiB → ceiling ${rawCeiling.toFixed(1)} / ${gzipCeiling.toFixed(1)} KiB; ` +
      `${firstLoad.size} chunks; largest ${(largest.raw / 1024).toFixed(1)} KiB)`,
  );
  if (rawKb > rawCeiling) {
    console.error(
      `  raw first-load grew to ${rawKb.toFixed(1)} KiB, over ${budget.recorded_raw_kb} recorded plus ${allowanceRaw} allowed`,
    );
    failed = true;
  }
  if (gzipKb > gzipCeiling) {
    console.error(
      `  gzip first-load grew to ${gzipKb.toFixed(1)} KiB, over ${budget.recorded_gzip_kb} recorded plus ${allowanceGzip} allowed`,
    );
    failed = true;
  }
  if (budget.max_single_chunk_kb && largest.raw / 1024 > budget.max_single_chunk_kb) {
    console.error(`  single chunk ${largest.chunk} is ${(largest.raw / 1024).toFixed(1)} KiB (max ${budget.max_single_chunk_kb})`);
    failed = true;
  }

  const forbidden = (contract.forbidden_in_first_load?.[route] ?? []).map((entry) =>
    typeof entry === 'string' ? { name: entry, markers: [entry] } : entry,
  );
  for (const entry of forbidden) {
    for (const { chunk, text } of contents) {
      if ((entry.markers ?? [entry.name]).some((marker) => text.includes(marker))) {
        console.error(`  forbidden library "${entry.name}" is in first-load chunk ${chunk}`);
        failed = true;
      }
    }
  }

  // Everything under static/chunks that is NOT in this route's first-load set
  // is lazy. Hashing renames chunks every build, so a large lazy chunk is
  // recorded by a stable content marker + reason, never by filename.
  const chunkDir = path.join(NEXT, 'static', 'chunks');
  const lazyChunks = fs.existsSync(chunkDir)
    ? fs.readdirSync(chunkDir).filter((name) => name.endsWith('.js')).map((name) => `static/chunks/${name}`)
    : [];
  const records = contract.recorded_lazy_chunks ?? [];
  const lazyText = new Map();
  for (const chunk of lazyChunks) {
    if (firstLoad.has(chunk)) continue;
    const file = path.join(chunkDir, path.basename(chunk));
    const size = fs.statSync(file).size;
    const text = fs.readFileSync(file, 'utf8');
    lazyText.set(chunk, text);
    if (budget.max_lazy_chunk_kb && size / 1024 > budget.max_lazy_chunk_kb) {
      const record = records.find((entry) => entry.marker && text.includes(entry.marker));
      if (!record) {
        console.error(
          `  lazy chunk ${chunk} is ${(size / 1024).toFixed(1)} KiB (max ${budget.max_lazy_chunk_kb}) and has no recorded_lazy_chunks marker`,
        );
        failed = true;
      }
    }
  }
  // A record whose marker no longer appears in any lazy chunk is stale and
  // must be deleted; otherwise the allowlist rots into a permission slip.
  for (const record of records) {
    if (!record.marker) {
      console.error('  recorded_lazy_chunks entry has no marker');
      failed = true;
      continue;
    }
    const present = [...lazyText.values()].some((text) => text.includes(record.marker));
    if (!present) {
      console.error(`  recorded_lazy_chunks marker "${record.marker}" (${record.reason ?? 'no reason'}) is stale: not found in any lazy chunk`);
      failed = true;
    }
  }
}

if (failed) process.exit(1);
console.log('console-bundle-budgets: all route budgets satisfied');
