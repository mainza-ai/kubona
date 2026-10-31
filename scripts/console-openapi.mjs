#!/usr/bin/env node
// Generate OpenAPI 3.1 for the modeled console endpoints from:
//   - platform/contracts/console-contract-inventory.json (paths/methods)
//   - platform/console/src/lib/responseContract.ts (response schemas)
//
// Unmodeled API functions are named explicitly at the document root; they are
// not silently presented as documented operations. Run --write to regenerate,
// --check to enforce freshness in CI.
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CONSOLE = path.join(ROOT, 'platform', 'console');
const INVENTORY = path.join(ROOT, 'platform', 'contracts', 'console-contract-inventory.json');
const CONTRACT = path.join(CONSOLE, 'src', 'lib', 'responseContract.ts');
const OUT = path.join(ROOT, 'platform', 'contracts', 'console-openapi.json');
const require = createRequire(path.join(CONSOLE, 'package.json'));
const ts = require('typescript');

function loadSchemas() {
  const source = fs.readFileSync(CONTRACT, 'utf8');
  const js = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  }).outputText;
  const mod = { exports: {} };
  const fn = new Function('exports', 'require', 'module', '__filename', '__dirname', js);
  fn(mod.exports, require, mod, CONTRACT, path.dirname(CONTRACT));
  if (!mod.exports.RESPONSE_SCHEMAS) throw new Error('RESPONSE_SCHEMAS missing');
  return mod.exports.RESPONSE_SCHEMAS;
}

function toJsonSchema(schema) {
  if (!schema || typeof schema !== 'object') return {};
  if (schema.type === 'any') return {};
  const out = { type: schema.type };
  if (schema.nullable) out.nullable = true;
  if (schema.required) out.required = schema.required;
  if (schema.properties) {
    out.properties = {};
    for (const [key, value] of Object.entries(schema.properties)) out.properties[key] = toJsonSchema(value);
  }
  if (schema.items) out.items = toJsonSchema(schema.items);
  return out;
}

function operationId(name) {
  return name.replace(/[^A-Za-z0-9_]/g, '_');
}

function main() {
  const inventory = JSON.parse(fs.readFileSync(INVENTORY, 'utf8'));
  const schemas = loadSchemas();
  const components = {};
  for (const [name, schema] of Object.entries(schemas)) components[name] = toJsonSchema(schema);

  const paths = {};
  const used = new Map();
  for (const fn of inventory.modeled_functions) {
    if (!fn.path || !fn.method) continue;
    const openapiPath = fn.path.replace(/:([A-Za-z0-9_]+)/g, '{$1}');
    paths[openapiPath] ??= {};
    const key = `${fn.method} ${openapiPath}`;
    if (used.has(key)) {
      const existing = used.get(key);
      if (existing !== fn.contract) {
        throw new Error(`console-openapi: ${key} has conflicting contracts ${existing} and ${fn.contract}`);
      }
      continue;
    }
    used.set(key, fn.contract);
    const params = [...openapiPath.matchAll(/\{([^}]+)\}/g)].map((m) => ({
      name: m[1],
      in: 'path',
      required: true,
      schema: { type: 'string' },
    }));
    paths[openapiPath][fn.method.toLowerCase()] = {
      operationId: operationId(fn.function),
      tags: [openapiPath.split('/')[2] || 'api'],
      ...(params.length ? { parameters: params } : {}),
      responses: {
        '200': {
          description: `Response contract: ${fn.contract}`,
          content: {
            'application/json': {
              schema: { $ref: `#/components/schemas/${fn.contract}` },
            },
          },
        },
      },
    };
  }

  const doc = {
    openapi: '3.1.0',
    info: {
      title: 'Kubona Console API',
      version: '1.0.0',
      description:
        'Generated from the console contract inventory. Only modeled operations are documented; unmodeled functions are listed explicitly.',
    },
    paths,
    components: { schemas: components },
    'x-kubona-unmodeled-functions': inventory.unmodeled_functions.map((f) => f.function),
  };
  const rendered = JSON.stringify(doc, null, 2) + '\n';
  if (process.argv.includes('--check')) {
    if (!fs.existsSync(OUT)) {
      console.error(`console-openapi: missing ${path.relative(ROOT, OUT)}`);
      process.exit(1);
    }
    if (fs.readFileSync(OUT, 'utf8') !== rendered) {
      console.error('console-openapi: stale; run node scripts/console-openapi.mjs --write');
      process.exit(1);
    }
    console.log(`console-openapi: ${Object.keys(paths).length} paths, ${Object.keys(components).length} schemas, ${doc['x-kubona-unmodeled-functions'].length} unmodeled`);
    return;
  }
  fs.writeFileSync(OUT, rendered);
  console.log(`console-openapi: wrote ${path.relative(ROOT, OUT)}`);
}

main();
