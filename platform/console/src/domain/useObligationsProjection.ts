'use client';

import { fetchOpenObligations, hasOperatorBearer, type ObligationRow } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { useEffect, useRef } from 'react';
import { useAuthPhase } from './auth-state';
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
  const authPhase = useAuthPhase();
  const prevAuthPhase = useRef(authPhase);
  const bearerConfigured = typeof hasOperatorBearer === 'function' && hasOperatorBearer();
  const authHalted =
    status === 'refused' &&
    failure?.kind === 'auth' &&
    !bearerConfigured &&
    authPhase !== 'authenticated';

  useEffect(() => {
    setPollMs(live || authHalted ? 0 : 15_000);
  }, [live, authHalted, setPollMs]);

  useEffect(() => {
    const previous = prevAuthPhase.current;
    prevAuthPhase.current = authPhase;
    if (authPhase === 'authenticated' && previous !== 'authenticated') {
      refresh();
    }
  }, [authPhase, refresh]);

  useEffect(() => subscribeInvalidation(['obligations'], refresh), [refresh]);
  return { obligations: data ?? [], status, error, failure, refresh };
}
