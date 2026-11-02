'use client';

import { fetchOpenObligations, type ObligationRow } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { useEffect } from 'react';
import { useSharedResource } from './shared-resource';
import { subscribeInvalidation } from './invalidation';
import { useSessionStreamLive } from './stream-state';

/** One page-level owner for the open obligation projection. */
export function useObligationsProjection() {
  const { data, status, error, failure, refresh, setPollMs } = useSharedResource<ObligationRow[]>(
    'obligations',
    async () => {
      const out = await fetchOpenObligations();
      if (!out.ok) throw new ApiFailureError(out.failure);
      return out.obligations;
    },
    { intervalMs: 15_000 },
  );
  const live = useSessionStreamLive();
  useEffect(() => { setPollMs(live ? 0 : 15_000); }, [live, setPollMs]);
  useEffect(() => subscribeInvalidation(['obligations'], refresh), [refresh]);
  return { obligations: data ?? [], status, error, failure, refresh };
}
