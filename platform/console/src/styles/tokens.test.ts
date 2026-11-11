import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const SRC_ROOT = fileURLToPath(new URL('..', import.meta.url));
const STYLES_DIR = fileURLToPath(new URL('.', import.meta.url));

function walkFiles(dir: string, extensions: readonly string[]): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      out.push(...walkFiles(full, extensions));
      continue;
    }
    if (full.includes('.test.') || full.includes('.spec.')) continue;
    if (extensions.some((ext) => full.endsWith(ext))) {
      out.push(full);
    }
  }
  return out;
}

function collectDeclaredColorTokens(): Set<string> {
  const declared = new Set<string>();
  const cssFiles = [
    join(STYLES_DIR, 'tokens.css'),
    join(STYLES_DIR, 'axes.css'),
    join(SRC_ROOT, 'app', 'globals.css'),
  ];
  const declRe = /(--color-[a-z0-9-]+)\s*:/g;
  for (const file of cssFiles) {
    const text = readFileSync(file, 'utf8');
    let match: RegExpExecArray | null;
    while ((match = declRe.exec(text)) !== null) {
      declared.add(match[1]);
    }
  }
  return declared;
}

// Allowed non-theme literal exceptions (audited intentionally):
// 1. Fixed fullscreen backdrop scrims behind floating modal/drawer overlays.
const FIXED_BACKDROP_SCRIM_RE = /\bfixed\s+inset-0\b[^"'`]*\bbg-black\/(60|70|75)\b/;
// 2. Runtime CSS-variable reader fallbacks like tok('--color-text', '#f4f4f6').
const TOK_FALLBACK_CALL_RE = /\btok\(\s*['"]--color-[a-z0-9-]+['"]\s*,\s*['"]#[0-9a-fA-F]{6}['"]\s*\)/;

const DISALLOWED_DARK_PATTERNS: Array<{ label: string; regex: RegExp }> = [
  { label: 'text-white', regex: /\btext-white\b/ },
  { label: 'text-slate-*', regex: /\btext-slate-\d+\b/ },
  { label: 'text-neutral-*', regex: /\btext-neutral-\d+\b/ },
  { label: 'text-zinc/gray-*', regex: /\b(?:text|bg)-(?:zinc|gray)-\d+\b/ },
  { label: 'bg-black', regex: /\bbg-black(?:\/\d+)?\b/ },
  { label: 'bg-slate/neutral-*', regex: /\bbg-(?:slate|neutral)-\d+(?:\/\d+)?\b/ },
  { label: 'dark-[#0...]', regex: /#0[0-9a-fA-F]{5}\b/ },
  { label: 'border-white/*', regex: /\bborder-white\/[\d[\].]+/ },
  { label: 'bg-white/*', regex: /\bbg-white\/[\d[\].]+/ },
  { label: 'bg-*-950', regex: /\bbg-(?:emerald|cyan|rose|amber|indigo|violet|red|slate)-950(?:\/\d+)?\b/ },
  { label: 'pale dark-mode status text', regex: /\btext-(?:emerald|cyan|rose|amber|indigo|violet|purple|red)-(?:100|200|300|400|500)\b/ },
];

describe('design token integrity and Light Paper theme migration', () => {
  const declared = collectDeclaredColorTokens();

  it('declares all required Light Paper and dark-mode semantic tokens', () => {
    const requiredTokens = [
      '--color-canvas',
      '--color-page',
      '--color-chrome',
      '--color-surface',
      '--color-surface-elev',
      '--color-raised',
      '--color-float',
      '--color-fill',
      '--color-fill-hover',
      '--color-card',
      '--color-tile',
      '--color-field',
      '--color-border',
      '--color-border-strong',
      '--color-text',
      '--color-label-secondary',
      '--color-muted',
      '--color-faint',
      '--color-accent',
      '--color-lit',
      '--color-on-accent',
      '--color-info-wash',
      '--color-info-edge',
      '--color-info-ink',
      '--color-success-wash',
      '--color-success-edge',
      '--color-success-ink',
      '--color-warning-wash',
      '--color-warning-edge',
      '--color-warning-ink',
      '--color-danger-wash',
      '--color-danger-edge',
      '--color-danger-ink',
      '--color-soft-lilac-wash',
      '--color-soft-lilac-edge',
      '--color-soft-lilac-ink',
      '--color-soft-sky-wash',
      '--color-soft-sky-edge',
      '--color-soft-sky-ink',
      '--color-soft-rose-wash',
      '--color-soft-rose-edge',
      '--color-soft-rose-ink',
    ];
    const missing = requiredTokens.filter((token) => !declared.has(token));
    expect(missing).toEqual([]);

    const axesCss = readFileSync(join(STYLES_DIR, 'axes.css'), 'utf8');
    const tokensCss = readFileSync(join(STYLES_DIR, 'tokens.css'), 'utf8');
    expect(tokensCss).toContain('--color-canvas: #1a1a1d');
    expect(axesCss).toMatch(/:root\[data-theme=["']light["']\]/);
    expect(axesCss).toContain('--color-canvas: #f1f1f3');
    expect(axesCss).toContain('--color-surface: #ffffff');
    expect(axesCss).toContain('--color-text: #1c1c1f');
  });

  it('resolves every var(--color-*) reference in src CSS and TSX/TS files', () => {
    const files = walkFiles(SRC_ROOT, ['.css', '.ts', '.tsx']);
    const refRe = /var\(\s*(--color-[a-z0-9-]+)\s*[,)]/g;
    const unresolved: Array<{ file: string; token: string }> = [];

    for (const file of files) {
      const text = readFileSync(file, 'utf8');
      let match: RegExpExecArray | null;
      while ((match = refRe.exec(text)) !== null) {
        const token = match[1];
        if (!declared.has(token)) {
          unresolved.push({ file: relative(SRC_ROOT, file), token });
        }
      }
    }

    expect(unresolved).toEqual([]);
  });

  it('keeps TSX/TS UI components free of un-tokenized dark-only utility classes', () => {
    const files = walkFiles(SRC_ROOT, ['.ts', '.tsx']);
    const violations: Array<{ file: string; line: number; rule: string; snippet: string }> = [];

    for (const file of files) {
      const lines = readFileSync(file, 'utf8').split('\n');
      lines.forEach((rawLine, index) => {
        const line = rawLine.trim();
        if (!line || line.startsWith('//') || line.startsWith('*')) return;
        const sanitized = line
          .replace(FIXED_BACKDROP_SCRIM_RE, '')
          .replace(TOK_FALLBACK_CALL_RE, '');

        for (const { label, regex } of DISALLOWED_DARK_PATTERNS) {
          if (regex.test(sanitized)) {
            violations.push({
              file: relative(SRC_ROOT, file),
              line: index + 1,
              rule: label,
              snippet: line.slice(0, 140),
            });
          }
        }
      });
    }

    expect(violations).toEqual([]);
  });
});
