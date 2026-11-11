'use client';

import { fetchOpenObligations, hasOperatorBearer, type ObligationRow } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { useEffect, useRef } from 'react';
import { shouldDeferProtectedRead, useAuthPhase, useMultiLabAuthRequired } from './auth-state';
import { useSharedResource } from './shared-resource';
import { subscribeInvalidation } from './invalidation';
import { useSessionStreamLive } from './stream-state';

/** One page-level owner for the open obligation projection. */
export function useObligationsProjection(enabled = true) {
  const bearerConfigured = typeof hasOperatorBearer === 'function' && hasOperatorBearer();
  const authPhase = useAuthPhase();
  const multiLabRequired = useMultiLabAuthRequired();
  const deferredAuth = !enabled || (multiLabRequired && authPhase !== 'authenticated' && !bearerConfigured) || shouldDeferProtectedRead(bearerConfigured);
  const { data, status, error, failure, refresh, setPollMs } = useSharedResource<ObligationRow[]>(
    'obligations',
    async () => {
      if (deferredAuth) return [];
      const out = await fetchOpenObligations();
      if (!out.ok) throw new ApiFailureError(out.failure);
      return out.obligations;
    },
    { intervalMs: 15_000 },
  );
  const live = useSessionStreamLive();
  const prevAuthPhase = useRef(authPhase);
  const prevDeferred = useRef(deferredAuth);
  const authHalted =
    status === 'refused' &&
    failure?.kind === 'auth' &&
    !bearerConfigured &&
    authPhase !== 'authenticated';

  useEffect(() => {
    setPollMs(live || authHalted || deferredAuth ? 0 : 15_000);
  }, [live, authHalted, deferredAuth, setPollMs]);

  useEffect(() => {
    const previousDeferred = prevDeferred.current;
    prevDeferred.current = deferredAuth;
    const previous = prevAuthPhase.current;
    prevAuthPhase.current = authPhase;
    if (!deferredAuth && (previousDeferred || (authPhase === 'authenticated' && previous !== 'authenticated'))) {
      refresh();
    }
  }, [authPhase, deferredAuth, refresh]);

  useEffect(() => subscribeInvalidation(['obligations'], refresh), [refresh]);
  return { obligations: data ?? [], status, error, failure, refresh };
}
