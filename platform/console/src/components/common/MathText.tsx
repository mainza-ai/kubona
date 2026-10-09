'use client';

import React, { useMemo } from 'react';
import katex from 'katex';

interface MathTextProps {
  math?: string;
  text?: string;
  block?: boolean;
  className?: string;
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
        return `<span class="text-red-400">${math}</span>`;
      }
    }

    if (text) {
      // Parse inline $...$ or display $$...$$ within text
      const regex = /(\$\$[\s\S]*?\$\$|\$[^\$]*?\$)/g;
      const parts = text.split(regex);
      return parts
        .map((part) => {
          if (part.startsWith('$$') && part.endsWith('$$')) {
            const raw = part.slice(2, -2);
            try {
              return katex.renderToString(raw, { displayMode: true, throwOnError: false });
            } catch {
              return part;
            }
          }
          if (part.startsWith('$') && part.endsWith('$')) {
            const raw = part.slice(1, -1);
            try {
              return katex.renderToString(raw, { displayMode: false, throwOnError: false });
            } catch {
              return part;
            }
          }
          return part;
        })
        .join('');
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
