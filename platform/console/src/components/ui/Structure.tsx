'use client';

import React from 'react';

export function Surface({
  children,
  className = '',
  ...props
}: React.HTMLAttributes<HTMLElement> & { children: React.ReactNode }) {
  return <section {...props} className={`k-surface ${className}`.trim()}>{children}</section>;
}

export function ListRow({
  children,
  className = '',
  ...props
}: React.HTMLAttributes<HTMLDivElement> & { children: React.ReactNode }) {
  return <div {...props} className={`k-list-row ${className}`.trim()}>{children}</div>;
}

export function Disclosure({
  summary,
  children,
  open,
  className = '',
}: {
  summary: React.ReactNode;
  children: React.ReactNode;
  open?: boolean;
  className?: string;
}) {
  return (
    <details className={`k-disclosure ${className}`.trim()} open={open}>
      <summary>{summary}</summary>
      <div className="k-disclosure-body">{children}</div>
    </details>
  );
}
