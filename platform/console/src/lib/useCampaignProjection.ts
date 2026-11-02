'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchCampaignResult, type ApiFailure } from './api';
import { subscribeInvalidation } from '@/domain/invalidation';
import type { CampaignState } from '@/types';

/**
 * Owns the campaign projection for one session. The server is authoritative;
 * stale responses from a previous session are discarded before they land.
 */
export function useCampaignProjection(sessionId: string | null) {
  const [campaign, setCampaign] = useState<CampaignState | null>(null);
  const [failure, setFailure] = useState<ApiFailure | null>(null);
  const generation = useRef(0);
  const newestAt = useRef(0);
  const current = useRef<CampaignState | null>(null);

  const apply = useCallback((next: CampaignState | null) => {
    if (next === null) {
      newestAt.current = 0;
      current.current = null;
      setCampaign(null);
      return;
    }
    const parsed = Date.parse(String(next.updated_at ?? ''));
    const at = Number.isFinite(parsed) ? parsed : 0;
    if (at > 0 && at < newestAt.current) return;
    if (at > 0) newestAt.current = at;
    current.current = next;
    setCampaign(next);
  }, []);

  const reload = useCallback(async (): Promise<CampaignState | null> => {
    const ticket = ++generation.current;
    if (!sessionId) {
      apply(null);
      setFailure(null);
      return null;
    }
    const result = await fetchCampaignResult(sessionId);
    if (ticket !== generation.current) return result.ok ? result.data : null;
    if (!result.ok) {
      setFailure(result.failure);
      return current.current;
    }
    setFailure(null);
    apply(result.data);
    return result.data;
  }, [sessionId, apply]);

  useEffect(() => subscribeInvalidation(['campaign'], () => { void reload(); }), [reload]);

  useEffect(() => {
    let cancelled = false;
    void Promise.resolve().then(() => {
      if (cancelled) return;
      if (!sessionId) {
        apply(null);
        setFailure(null);
        return;
      }
      void reload();
    });
    return () => {
      cancelled = true;
      generation.current += 1;
    };
  }, [sessionId, reload, apply]);

  return {
    campaign,
    setCampaign: apply,
    reload,
    failure,
    status: failure ? 'refused' as const : campaign ? 'ready' as const : 'idle' as const,
  };
}
