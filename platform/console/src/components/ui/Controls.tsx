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
  const refs = React.useRef<Array<HTMLButtonElement | null>>([]);
  const enabled = segments.filter((segment) => !segment.disabled);

  // Radiogroup keyboard contract: arrows move selection, Home/End jump, and
  // only the checked radio is in the tab order (roving tabindex).
  const move = (delta: number) => {
    if (enabled.length === 0) return;
    const current = enabled.findIndex((segment) => segment.value === value);
    const base = current < 0 ? 0 : current;
    const next = enabled[(base + delta + enabled.length) % enabled.length];
    onChange(next.value);
    const index = segments.findIndex((segment) => segment.value === next.value);
    refs.current[index]?.focus();
  };
  const onKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        event.preventDefault();
        move(1);
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        event.preventDefault();
        move(-1);
        break;
      case 'Home':
        event.preventDefault();
        if (enabled.length > 0) {
          onChange(enabled[0].value);
          refs.current[segments.findIndex((segment) => segment.value === enabled[0].value)]?.focus();
        }
        break;
      case 'End':
        event.preventDefault();
        if (enabled.length > 0) {
          const last = enabled[enabled.length - 1];
          onChange(last.value);
          refs.current[segments.findIndex((segment) => segment.value === last.value)]?.focus();
        }
        break;
      default:
        break;
    }
  };

  return (
    <div className={`k-segmented ${className}`.trim()} role="radiogroup" aria-label={label}>
      {segments.map((segment, index) => (
        <button
          key={segment.value}
          ref={(node) => { refs.current[index] = node; }}
          type="button"
          role="radio"
          aria-checked={value === segment.value}
          tabIndex={value === segment.value ? 0 : -1}
          disabled={segment.disabled}
          className={`k-segmented-item ${value === segment.value ? 'is-selected' : ''}`.trim()}
          onClick={() => onChange(segment.value)}
          onKeyDown={onKeyDown}
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
