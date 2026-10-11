'use client';

import React, { useMemo } from 'react';
import katex from 'katex';

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

function renderMath(raw: string, displayMode: boolean): string {
  try {
    return katex.renderToString(raw, { displayMode, throwOnError: false });
  } catch {
    return escapeHtml(displayMode ? `$$${raw}$$` : `$${raw}$`);
  }
}

// Extract $$..$$ / $..$ spans to placeholders so markdown formatting never
// touches math contents. Returns [protected text, restore fn].
function protectMath(text: string): { text: string; restore: (s: string) => string } {
  const slots: string[] = [];
  const protected_ = text.replace(/\$\$[\s\S]*?\$\$|\$[^\$]*?\$/g, (m) => {
    let html: string;
    if (m.startsWith('$$')) {
      html = renderMath(m.slice(2, -2), true);
    } else {
      html = renderMath(m.slice(1, -1), false);
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
  out = out.replace(/`([^`]+?)`/g, '<code class="px-1 rounded bg-white/10 font-code text-[11px]">$1</code>');
  out = out.replace(/\*\*([^*]+?)\*\*/g, '<strong class="font-semibold text-white">$1</strong>');
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

function renderMarkdownText(text: string): string {
  // Block parsing runs on raw (math-protected) lines; every emitted text
  // fragment is escaped at the boundary via inlineRaw / escapeHtml, so raw
  // <, >, & from model output can never reach the DOM as markup.
  const { text: sheltered, restore } = protectMath(text);
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
        `<pre class="my-2 p-2.5 rounded-xl bg-black/50 border border-white/10 overflow-x-auto font-code text-[11px] text-slate-200">${escapeHtml(code.join('\n'))}</pre>`
      );
      continue;
    }

    const header = /^(#{1,4})\s+(.*)$/.exec(trim);
    if (header) {
      flushPara();
      const level = header[1].length;
      blocks.push(
        `<div class="mt-2 mb-1 font-semibold text-white ${level <= 2 ? 'text-[13px]' : 'text-xs'}">${inlineRaw(header[2])}</div>`
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
      const th = headerCells.map((c) => `<th class="px-2 py-1 text-left font-semibold text-slate-100 border-b border-white/10">${inlineRaw(c)}</th>`).join('');
      const tb = rows
        .map(
          (r) =>
            `<tr class="border-b border-white/5">${r.map((c) => `<td class="px-2 py-1 text-slate-300">${inlineRaw(c)}</td>`).join('')}</tr>`
        )
        .join('');
      blocks.push(
        `<div class="my-2 overflow-x-auto rounded-xl border border-white/10"><table class="w-full text-[11px] border-collapse"><thead><tr>${th}</tr></thead><tbody>${tb}</tbody></table></div>`
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
        `<ul class="my-1.5 space-y-1">${items.map((it) => `<li class="flex gap-1.5"><span class="text-cyan-400">•</span><span>${inlineRaw(it)}</span></li>`).join('')}</ul>`
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
        `<ol class="my-1.5 space-y-1 list-decimal list-inside marker:text-cyan-400">${items.map((it) => `<li>${inlineRaw(it)}</li>`).join('')}</ol>`
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
        `<div class="my-1.5 pl-2.5 border-l-2 border-cyan-400/40 text-slate-300">${parts.map(renderInline).join('<br/>')}</div>`
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
  const html = useMemo(() => {
    if (math) {
      try {
        return katex.renderToString(math, {
          displayMode: block,
          throwOnError: false,
        });
      } catch {
        return `<span class="text-red-400">${escapeHtml(math)}</span>`;
      }
    }

    if (text) {
      return renderMarkdownText(text);
    }

    return '';
  }, [math, text, block]);

  return (
    <span
      className={`inline-block ${className}`}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
