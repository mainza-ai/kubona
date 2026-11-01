'use client';

import { fetchOpenObligations, type ObligationRow } from '@/lib/api';
import { useSharedResource } from './shared-resource';

/** One page-level owner for the open obligation projection. */
export function useObligationsProjection() {
  const { data, status, error, refresh } = useSharedResource<ObligationRow[]>(
    'obligations',
    async () => {
      const out = await fetchOpenObligations();
      if (!out.ok) throw new Error(out.error);
      return out.obligations;
    },
    { intervalMs: 15_000 },
  );
  return { obligations: data ?? [], status, error, refresh };
}
