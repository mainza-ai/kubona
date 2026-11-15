'use client';

import dynamic from 'next/dynamic';
import React from 'react';
import type { ChartPoint, ChartSpec } from './chart';
import { LazyBoundary } from './LazyBoundary';

// Charts are a lazy boundary: surfaces that never show a figure must not pay
// for the chart renderer in their first client chunk. The retry boundary sits
// outside the dynamic load so a stale chunk gets a real reload path.
const DynamicChart = dynamic(
  () => import('./DataChart').then((module) => module.Chart),
  {
    ssr: false,
    loading: () => (
      <div className="k-chart k-chart-loading" role="status">
        Loading figure…
      </div>
    ),
  },
);

export type ChartProps = {
  spec: ChartSpec;
  className?: string;
  embedded?: boolean;
  onEvidence?: (source: NonNullable<ChartPoint['evidence']>[number]) => void;
};

export function Chart(props: ChartProps) {
  return (
    <LazyBoundary label="Figure">
      <DynamicChart {...props} />
    </LazyBoundary>
  );
}
