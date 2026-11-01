'use client';

import { fetchCostSummary, type CostSummary } from '@/lib/api';
import { usePollingResource } from './use-polling-resource';

export function useCostProjection() {
  return usePollingResource<CostSummary>(
    async () => {
      const out = await fetchCostSummary();
      if (!out.ok) throw new Error(out.error);
      return out.result;
    },
    { intervalMs: 60_000 },
  );
}
