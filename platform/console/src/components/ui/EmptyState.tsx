'use client';

import React from 'react';

/** Shared empty state: names what is absent; never invents a default. */
export function EmptyState({
  title,
  children,
  className = '',
}: {
  title: string;
  children?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={`k-empty-state ${className}`.trim()} role="status">
      <strong>{title}</strong>
      {children && <span>{children}</span>}
    </div>
  );
}
