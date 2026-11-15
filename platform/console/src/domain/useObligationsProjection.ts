'use client';

import { fetchOpenObligations, hasOperatorBearer, type ObligationRow } from '@/lib/api';
import { ApiFailureError } from '@/lib/api-errors';
import { useEffect, useRef } from 'react';
import { shouldDeferProtectedRead, useAuthPhase, useMultiLabAuthRequired } from './auth-state';
import { useSharedResource } from './shared-resource';
import { subscribeInvalidation } from './invalidation';

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
  const prevAuthPhase = useRef(authPhase);
  const prevDeferred = useRef(deferredAuth);
  const authHalted =
    status === 'refused' &&
    failure?.kind === 'auth' &&
    !bearerConfigured &&
    authPhase !== 'authenticated';

  useEffect(() => {
    // Obligations are cross-session: the tray shows the whole lab's queue,
    // while /api/events/stream is session-scoped and carries no obligation
    // events. So the poll stays alive even when that stream is live —
    // disabling it would silently freeze the reviewer's inbox. A local
    // obligation_* ledger event still refreshes instantly via invalidation.
    setPollMs(authHalted || deferredAuth ? 0 : 15_000);
  }, [authHalted, deferredAuth, setPollMs]);

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
