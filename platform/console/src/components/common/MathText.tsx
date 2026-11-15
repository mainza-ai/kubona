'use client';

import React, { useEffect, useMemo, useState } from 'react';

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
    return katex.renderToString(raw, { displayMode, throwOnError: false });
  } catch {
    return escapeHtml(displayMode ? `$$${raw}$$` : `$${raw}$`);
  }
}

// Extract $$..$$ / $..$ spans to placeholders so markdown formatting never
// touches math contents, while leaving fenced/inline code spans untouched.
function protectMath(text: string, katex: KatexRenderer | null): { text: string; restore: (s: string) => string } {
  const slots: string[] = [];
  const protected_ = text.replace(/```[\s\S]*?```|`[^`\n]+`|\$\$[\s\S]*?\$\$|\$[^$\n]*?\$/g, (m) => {
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

function renderInline(s: string): string {
  // s is already HTML-escaped; placeholders contain NUL chars (safe).
  let out = s;
  out = out.replace(/`([^`]+?)`/g, '<code class="px-1 rounded-chip bg-fill text-text font-code text-[12px]">$1</code>');
  out = out.replace(/\*\*([^*]+?)\*\*/g, '<strong class="font-semibold text-text">$1</strong>');
  out = out.replace(/(^|[^*\w])\*([^*\n]+?)\*(?![*\w])/g, '$1<em>$2</em>');
  return out;
}

// inlineRaw escapes raw model prose (placeholders survive) then formats it.
function inlineRaw(s: string): string {
  return renderInline(escapeHtml(s));
}

function isTableDivider(line: string): boolean {
  const cells = line.split('|').map((c) => c.trim()).filter(Boolean);
  return cells.length > 0 && cells.every((c) => /^:?-+:?$/.test(c));
}

function renderMarkdownText(text: string, katex: KatexRenderer | null): string {
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
      blocks.push(`<p class="my-1.5 leading-relaxed">${para.map(inlineRaw).join('<br/>')}</p>`);
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
        `<div class="mt-2 mb-1 font-semibold text-text ${level <= 2 ? 'text-[13px]' : 'text-label'}">${inlineRaw(header[2])}</div>`
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
      const th = headerCells.map((c) => `<th class="px-2 py-1 text-left font-semibold text-text border-b border-border">${inlineRaw(c)}</th>`).join('');
      const tb = rows
        .map(
          (r) =>
            `<tr class="border-b border-border">${r.map((c) => `<td class="px-2 py-1 text-label-secondary">${inlineRaw(c)}</td>`).join('')}</tr>`
        )
        .join('');
      blocks.push(
        `<div class="my-2 overflow-x-auto rounded-row border border-border"><table class="w-full text-[12px] border-collapse"><thead><tr>${th}</tr></thead><tbody>${tb}</tbody></table></div>`
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
        `<ul class="my-1.5 space-y-1">${items.map((it) => `<li class="flex gap-1.5"><span class="text-accent">•</span><span>${inlineRaw(it)}</span></li>`).join('')}</ul>`
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
        `<ol class="my-1.5 space-y-1 list-decimal list-inside marker:text-accent">${items.map((it) => `<li>${inlineRaw(it)}</li>`).join('')}</ol>`
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
        `<div class="my-1.5 pl-2.5 border-l-2 border-accent-edge text-label-secondary">${parts.map(inlineRaw).join('<br/>')}</div>`
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

export function MathText({ math, text, block = false, className = '' }: MathTextProps) {
  const needsKatex = Boolean(math) || Boolean(text?.includes('$'));
  const [katex, setKatex] = useState<KatexRenderer | null>(null);
  const [katexFailed, setKatexFailed] = useState(false);

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
        });
      } catch {
        return `<span class="text-danger-ink">${escapeHtml(math)}</span>`;
      }
    }

    if (text) {
      return renderMarkdownText(text, katex);
    }

    return '';
  }, [math, text, block, katex]);

  return (
    <span
      className={`inline-block ${className}`}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
