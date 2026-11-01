'use client';

import React from 'react';

/** Compact semantic label. Tone is presentation only. */
export function Badge({
  children,
  tone = 'neutral',
  className = '',
}: {
  children: React.ReactNode;
  tone?: 'neutral' | 'info' | 'success' | 'warning' | 'danger';
  className?: string;
}) {
  return <span className={`k-badge k-badge-${tone} ${className}`.trim()}>{children}</span>;
}
