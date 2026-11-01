'use client';

import { fetchCostSummary, type CostSummary } from '@/lib/api';
import { useSharedResource } from './shared-resource';

export function useCostProjection() {
  return useSharedResource<CostSummary>(
    'cost',
    async () => {
      const out = await fetchCostSummary();
      if (!out.ok) throw new Error(out.error);
      return out.result;
    },
    { intervalMs: 60_000 },
  );
}
