'use client';
'use memo'; // PERF-11: opt this hot renderer into the React Compiler.

import React, { useEffect, useMemo, useRef, useState } from 'react';

// D7: KaTeX must not enter the first-load bundle. It is ~280 KiB of renderer
// code and only chat prose that actually contains `$…$` needs it. The lazy
// import below creates a separate chunk; a message with no math never loads
// it, and an unresolved import fails closed to the escaped source text.
type KatexRenderer = typeof import('katex')['default'];

let katexPromise: Promise<KatexRenderer> | null = null;

function loadKatex(): Promise<KatexRenderer> {
  katexPromise ??= import('katex').then((mod) => mod.default ?? (mod as unknown as KatexRenderer));
  return katexPromise;
}

interface MathTextProps {
  math?: string;
  text?: string;
  block?: boolean;
  className?: string;
  /** B5: unverified numeric claims to mark inline in prose and table cells. */
  highlightNumbers?: string[];
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderMath(raw: string, displayMode: boolean, katex: KatexRenderer | null): string {
  if (!katex) {
    return escapeHtml(displayMode ? `$$${raw}$$` : `$${raw}$`);
  }
  try {
    return katex.renderToString(raw, {
      displayMode,
      throwOnError: false,
      trust: false,
      maxExpand: 200,
      maxSize: 10,
    });
  } catch {
    return escapeHtml(displayMode ? `$$${raw}$$` : `$${raw}$`);
  }
}

// Extract $$..$$ / $..$ spans to placeholders so markdown formatting never
// touches math contents, while leaving fenced/inline code spans untouched.
function protectMath(text: string, katex: KatexRenderer | null): { text: string; restore: (s: string) => string } {
  const slots: string[] = [];
  // Strip NUL bytes first so model/user prose cannot forge \u0000MATH0\u0000 placeholders.
  const sanitized = text.replace(/\u0000/g, '');
  const protected_ = sanitized.replace(/```[\s\S]*?```|`[^`\n]+`|\$\$[\s\S]*?\$\$|\$[^$\n]*?\$/g, (m) => {
    if (m.startsWith('`')) return m;
    let html: string;
    if (m.startsWith('$$')) {
      html = renderMath(m.slice(2, -2), true, katex);
    } else {
      html = renderMath(m.slice(1, -1), false, katex);
    }
    slots.push(html);
    return `\u0000MATH${slots.length - 1}\u0000`;
  });
  return {
    text: protected_,
    restore: (s: string) => s.replace(/\u0000MATH(\d+)\u0000/g, (_, i) => slots[Number(i)] ?? ''),
  };
}

function stripMathPlaceholders(s: string): string {
  return s.replace(/\u0000MATH\d+\u0000/g, 'math');
}

function renderInline(s: string): string {
  // s is already HTML-escaped; placeholders contain NUL chars (safe).
  let out = s;
  out = out.replace(/`([^`]+?)`/g, '<code class="px-1 rounded-chip bg-fill text-text font-code text-[12px]">$1</code>');
  out = out.replace(/\*\*([^*]+?)\*\*/g, '<strong class="font-semibold text-text">$1</strong>');
  out = out.replace(/(^|[^*\w])\*([^*\n]+?)\*(?![*\w])/g, '$1<em>$2</em>');
  return out;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function markNumbers(escaped: string, numbers: string[]): string {
  const unique = Array.from(new Set(numbers.filter(Boolean))).sort((a, b) => b.length - a.length);
  if (unique.length === 0) return escaped;
  const combined = new RegExp(`(^|[^\\w.])(${unique.map(escapeRegExp).join('|')})(?![\\w.])`, 'g');
  // Single-pass alternation outside math placeholders so a shorter number
  // can never match inside an already-emitted <mark> tag.
  return escaped
    .split(/(\u0000MATH\d+\u0000)/g)
    .map((part) => {
      if (part.startsWith('\u0000MATH')) return part;
      return part.replace(combined, `$1<mark class="rounded-chip bg-warning-wash px-0.5 text-warning-ink">$2</mark>`);
    })
    .join('');
}

// inlineRaw escapes raw model prose (placeholders survive) then formats it.
function inlineRaw(s: string, highlights: string[] = []): string {
  return renderInline(markNumbers(escapeHtml(s), highlights));
}

function isTableDivider(line: string): boolean {
  const cells = line.split('|').map((c) => c.trim()).filter(Boolean);
  return cells.length > 0 && cells.every((c) => /^:?-+:?$/.test(c));
}

function isNumericCell(value: string): boolean {
  return /^[+\-−]?\d+(?:\.\d+)?(?:\s|$)/.test(value.trim());
}

function formatNumericCell(value: string): { text: string; raw?: string } {
  const match = /^([+\-−]?\d+(?:\.\d+)?)([\s\S]*)$/.exec(value.trim());
  if (!match) return { text: value };
  const numeric = match[1];
  // Leave ordinary precision alone; only abbreviate values wide enough to
  // force a horizontal scroll. The untouched value stays in the title.
  if (numeric.replace(/\D/g, '').length <= 10) return { text: value };
  const parsed = Number(numeric.replace('−', '-'));
  if (!Number.isFinite(parsed)) return { text: value };
  const fixed = parsed.toFixed(6).replace(/0+$/, '').replace(/\.$/, '');
  return { text: `${fixed}${match[2]}`, raw: value };
}

function renderMarkdownText(text: string, katex: KatexRenderer | null, highlights: string[] = []): string {
  // Block parsing runs on raw (math-protected) lines; every emitted text
  // fragment is escaped at the boundary via inlineRaw / escapeHtml, so raw
  // <, >, & from model output can never reach the DOM as markup.
  const { text: sheltered, restore } = protectMath(text, katex);
  const lines = sheltered.split('\n');
  const blocks: string[] = [];
  let i = 0;
  let para: string[] = [];
  const flushPara = () => {
    if (para.length > 0) {
      blocks.push(`<p class="my-1.5 leading-relaxed">${para.map((line) => inlineRaw(line, highlights)).join('<br/>')}</p>`);
      para = [];
    }
  };

  while (i < lines.length) {
    const line = lines[i];
    const trim = line.trim();

    if (trim.startsWith('```')) {
      flushPara();
      const code: string[] = [];
      i += 1;
      while (i < lines.length && !lines[i].trim().startsWith('```')) {
        code.push(lines[i]);
        i += 1;
      }
      i += 1; // closing fence
      blocks.push(
        `<pre class="my-2 p-2.5 rounded-row bg-field border border-border overflow-x-auto font-code text-[12px] text-text">${escapeHtml(code.join('\n'))}</pre>`
      );
      continue;
    }

    const header = /^(#{1,4})\s+(.*)$/.exec(trim);
    if (header) {
      flushPara();
      const level = header[1].length;
      blocks.push(
        `<div class="mt-2 mb-1 font-semibold text-text ${level <= 2 ? 'text-[13px]' : 'text-label'}">${inlineRaw(header[2], highlights)}</div>`
      );
      i += 1;
      continue;
    }

    if (trim.startsWith('|') && i + 1 < lines.length && isTableDivider(lines[i + 1])) {
      flushPara();
      const headerCells = trim.split('|').map((c) => c.trim()).filter(Boolean);
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && lines[i].trim().startsWith('|')) {
        rows.push(lines[i].trim().split('|').map((c) => c.trim()).filter(Boolean));
        i += 1;
      }
      const numericColumns = headerCells.map(
        (_, index) => rows.length > 0 && rows.every((row) => isNumericCell(row[index] ?? '')),
      );
      const th = headerCells
        .map(
          (c, index) =>
            `<th scope="col" class="px-2 py-1 ${numericColumns[index] ? 'text-right' : 'text-left'} font-semibold text-text border-b border-border">${inlineRaw(c, highlights)}</th>`,
        )
        .join('');
      const tb = rows
        .map((r) => {
          const cells = r.map((c, index) => {
            const numeric = numericColumns[index] || isNumericCell(c);
            const formatted = formatNumericCell(c);
            const title = formatted.raw ? ` title="${escapeHtml(stripMathPlaceholders(formatted.raw))}"` : '';
            return `<td${title} class="px-2 py-1 align-top ${numeric ? 'text-right tabular-nums whitespace-nowrap' : 'text-label-secondary'}">${inlineRaw(formatted.text, highlights)}</td>`;
          }).join('');
          return `<tr class="border-b border-border">${cells}</tr>`;
        })
        .join('');
      const label = `Table: ${headerCells.map(stripMathPlaceholders).join(' · ')}`;
      blocks.push(
        `<div class="my-2 max-w-full min-w-0 rounded-row border border-border" style="max-width:100%;min-width:0;width:100%" data-table-shell><div class="flex items-center justify-between gap-2 border-b border-border px-2 py-1"><span class="text-[11px] font-code text-muted">${escapeHtml(label)}</span><span role="button" tabindex="0" data-copy-table class="cursor-pointer select-none rounded-control-compact border border-border px-1.5 py-0.5 text-caption font-code text-muted hover:bg-fill hover:text-text focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent" aria-label="${escapeHtml(`Copy ${label}`)}">Copy</span></div><div class="max-w-full min-w-0 overflow-x-auto scrollbar-thin" style="max-width:100%;min-width:0;overflow-x:auto" role="region" aria-label="${escapeHtml(label)}" tabindex="0"><table style="width:max-content;min-width:${headerCells.length >= 3 ? '30rem' : '100%'}" class="text-[12px] border-collapse"><thead><tr>${th}</tr></thead><tbody>${tb}</tbody></table></div></div>`
      );
      continue;
    }

    const bullet = /^(\s*)[-*•]\s+(.*)$/.exec(line);
    if (bullet) {
      flushPara();
      const items: string[] = [bullet[2]];
      i += 1;
      while (i < lines.length) {
        const m = /^(\s*)[-*•]\s+(.*)$/.exec(lines[i]);
        if (!m) break;
        items.push(m[2]);
        i += 1;
      }
      blocks.push(
        `<ul class="my-1.5 space-y-1">${items.map((it) => `<li class="flex gap-1.5"><span class="text-accent">•</span><span>${inlineRaw(it, highlights)}</span></li>`).join('')}</ul>`
      );
      continue;
    }

    const ordered = /^\s*\d+[.)]\s+(.*)$/.exec(line);
    if (ordered) {
      flushPara();
      const items: string[] = [ordered[1]];
      i += 1;
      while (i < lines.length) {
        const m = /^\s*\d+[.)]\s+(.*)$/.exec(lines[i]);
        if (!m) break;
        items.push(m[1]);
        i += 1;
      }
      blocks.push(
        `<ol class="my-1.5 space-y-1 list-decimal list-inside marker:text-accent">${items.map((it) => `<li>${inlineRaw(it, highlights)}</li>`).join('')}</ol>`
      );
      continue;
    }

    const quote = /^\s*>\s?(.*)$/.exec(line);
    if (quote) {
      flushPara();
      const parts: string[] = [quote[1]];
      i += 1;
      while (i < lines.length) {
        const m = /^\s*>\s?(.*)$/.exec(lines[i]);
        if (!m) break;
        parts.push(m[1]);
        i += 1;
      }
      blocks.push(
        `<div class="my-1.5 pl-2.5 border-l-2 border-accent-edge text-label-secondary">${parts.map((part) => inlineRaw(part, highlights)).join('<br/>')}</div>`
      );
      continue;
    }

    if (trim === '') {
      flushPara();
      i += 1;
      continue;
    }

    para.push(line.trim());
    i += 1;
  }
  flushPara();
  return restore(blocks.join(''));
}

export function MathText({ math, text, block = false, className = '', highlightNumbers = [] }: MathTextProps) {
  const needsKatex = Boolean(math) || Boolean(text?.includes('$'));
  const [katex, setKatex] = useState<KatexRenderer | null>(null);
  const [katexFailed, setKatexFailed] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!needsKatex || katex !== null || katexFailed) return;
    let cancelled = false;
    loadKatex()
      .then((renderer) => {
        if (!cancelled) setKatex(renderer);
      })
      .catch(() => {
        if (!cancelled) setKatexFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [needsKatex, katex, katexFailed]);

  const html = useMemo(() => {
    if (math) {
      if (!katex) {
        return `<span class="text-label-secondary">${escapeHtml(block ? `$$${math}$$` : `$${math}$`)}</span>`;
      }
      try {
        return katex.renderToString(math, {
          displayMode: block,
          throwOnError: false,
          trust: false,
          maxExpand: 200,
          maxSize: 10,
        });
      } catch {
        return `<span class="text-danger-ink">${escapeHtml(math)}</span>`;
      }
    }

    if (text) {
      return renderMarkdownText(text, katex, highlightNumbers);
    }

    return '';
  }, [math, text, block, katex, highlightNumbers]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const copyTable = (button: HTMLElement) => {
      const table = button.closest('[data-table-shell]')?.querySelector('table');
      if (!table) return;
      const text = Array.from(table.querySelectorAll('tr'))
        .map((row) => Array.from(row.querySelectorAll('th,td')).map((cell) => cell.textContent?.trim() ?? '').join('\t'))
        .join('\n');
      const clipboard = navigator.clipboard;
      if (!clipboard) return;
      void clipboard.writeText(text).then(() => {
        const previous = button.textContent;
        button.textContent = 'Copied';
        window.setTimeout(() => { button.textContent = previous ?? 'Copy'; }, 1200);
      }).catch(() => {
        // Clipboard denial is non-fatal; the table remains readable/selectable.
      });
    };
    const targetButton = (event: Event): HTMLElement | null => {
      const target = event.target;
      if (!(target instanceof HTMLElement)) return null;
      const button = target.closest('[data-copy-table]');
      return button instanceof HTMLElement && root.contains(button) ? button : null;
    };
    const onClick = (event: MouseEvent) => {
      const button = targetButton(event);
      if (button) copyTable(button);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Enter' && event.key !== ' ') return;
      const button = targetButton(event);
      if (!button) return;
      event.preventDefault();
      copyTable(button);
    };
    root.addEventListener('click', onClick);
    root.addEventListener('keydown', onKeyDown);
    return () => {
      root.removeEventListener('click', onClick);
      root.removeEventListener('keydown', onKeyDown);
    };
  }, [html]);

  return (
    <div
      ref={rootRef}
      className={`${math ? 'inline-block' : 'block min-w-0 max-w-full break-words'} ${className}`}
      style={math ? undefined : { minWidth: 0, maxWidth: '100%' }}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
