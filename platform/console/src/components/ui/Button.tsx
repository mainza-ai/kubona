'use client';

import React from 'react';

type Props = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  size?: 'compact' | 'regular';
  /** Busy state: disables the control and announces it via aria-busy. */
  pending?: boolean;
  /** Shorthand for variant="danger". */
  danger?: boolean;
};

/**
 * Shared semantic button. Presentation only: owners keep state and settlement.
 * `type` defaults to "button" so a button inside a form can never submit it by
 * accident; consequential form submits must opt in explicitly.
 */
export function Button({
  variant = 'secondary',
  size = 'regular',
  pending = false,
  danger = false,
  className = '',
  disabled,
  type = 'button',
  children,
  ...props
}: Props) {
  return (
    <button
      {...props}
      type={type}
      disabled={disabled || pending}
      aria-busy={pending || undefined}
      data-pending={pending ? 'true' : undefined}
      className={`k-button k-button-${danger ? 'danger' : variant} k-button-${size} ${className}`.trim()}
    >
      {pending ? (
        <>
          <span className="k-button-spinner" aria-hidden="true" />
          {children}
        </>
      ) : (
        children
      )}
    </button>
  );
}
