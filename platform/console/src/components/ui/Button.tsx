'use client';

import React from 'react';

type Props = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost';
  size?: 'compact' | 'regular';
};

/** Shared semantic button. Presentation only: owners keep state and settlement. */
export function Button({ variant = 'secondary', size = 'regular', className = '', ...props }: Props) {
  return (
    <button
      {...props}
      className={`k-button k-button-${variant} k-button-${size} ${className}`.trim()}
    />
  );
}
