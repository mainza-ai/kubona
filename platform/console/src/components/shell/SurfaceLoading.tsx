'use client';

import React from 'react';

/** Non-blank fallback for a lazy surface chunk. */
export function SurfaceLoading({ label = 'surface' }: { label?: string }) {
  return (
    <div role="status" className="flex flex-1 items-center justify-center p-8 text-sm text-slate-400">
      Loading {label}…
    </div>
  );
}
