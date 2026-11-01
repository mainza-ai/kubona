'use client';

import dynamic from 'next/dynamic';

// Charts are a lazy boundary: surfaces that never show a figure must not pay
// for the chart renderer in their first client chunk.
export const Chart = dynamic(
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
