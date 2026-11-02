'use client';

import React from 'react';

export function IconButton({
  label,
  className = '',
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { label: string }) {
  return (
    <button
      {...props}
      type={props.type ?? 'button'}
      aria-label={label}
      title={label}
      className={`k-button k-button-secondary k-button-compact k-icon-button ${className}`.trim()}
    />
  );
}

type Segment<T extends string> = { value: T; label: string; disabled?: boolean };

export function SegmentedControl<T extends string>({
  label,
  value,
  onChange,
  segments,
  className = '',
}: {
  label: string;
  value: T;
  onChange: (value: T) => void;
  segments: Segment<T>[];
  className?: string;
}) {
  return (
    <div className={`k-segmented ${className}`.trim()} role="radiogroup" aria-label={label}>
      {segments.map((segment) => (
        <button
          key={segment.value}
          type="button"
          role="radio"
          aria-checked={value === segment.value}
          disabled={segment.disabled}
          className={`k-segmented-item ${value === segment.value ? 'is-selected' : ''}`.trim()}
          onClick={() => onChange(segment.value)}
        >
          {segment.label}
        </button>
      ))}
    </div>
  );
}

export function Progress({
  label,
  value,
  max = 100,
  className = '',
}: {
  label: string;
  value: number;
  max?: number;
  className?: string;
}) {
  const bounded = Number.isFinite(value) ? Math.min(Math.max(0, value), max) : 0;
  return (
    <div className={`k-progress ${className}`.trim()}>
      <span className="k-progress-label">{label}</span>
      <progress value={bounded} max={max} aria-label={label} />
    </div>
  );
}

export function KeyHint({ children }: { children: React.ReactNode }) {
  return <kbd className="k-key-hint">{children}</kbd>;
}

export function Separator({ vertical = false, className = '' }: { vertical?: boolean; className?: string }) {
  return (
    <div
      role="separator"
      aria-orientation={vertical ? 'vertical' : 'horizontal'}
      className={`k-separator ${vertical ? 'k-separator-vertical' : ''} ${className}`.trim()}
    />
  );
}
