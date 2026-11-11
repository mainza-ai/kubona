'use client';

import React from 'react';

interface TipProps {
  label: string;
  sub?: string;
  position?: 'top' | 'bottom' | 'left' | 'right';
  children: React.ReactNode;
}

const POSITION_CLASS: Record<NonNullable<TipProps['position']>, string> = {
  top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
  bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
  left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  right: 'left-full top-1/2 -translate-y-1/2 ml-2',
};

/**
 * Rich hover tooltip in dock language: dark card, label + optional sub-line,
 * appears on hover only. For icon-only controls — text buttons carry their
 * own visible labels and need no tip.
 */
export function Tip({ label, sub, position = 'top', children }: TipProps) {
  return (
    <span className="group/tip relative inline-flex items-center justify-center">
      {children}
      <span
        role="tooltip"
        className={`absolute ${POSITION_CLASS[position]} px-2.5 py-1.5 rounded-lg bg-[var(--color-float)] border border-[var(--color-border-strong)] text-xs text-[var(--color-text)] whitespace-nowrap opacity-0 pointer-events-none group-hover/tip:opacity-100 transition-opacity duration-200 z-50 shadow-[var(--shadow-float)] text-left`}
      >
        <span className="font-medium block">{label}</span>
        {sub && <span className="block text-[12px] text-[var(--color-muted)]">{sub}</span>}
      </span>
    </span>
  );
}
