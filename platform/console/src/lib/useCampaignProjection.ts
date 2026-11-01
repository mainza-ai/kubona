'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchCampaignState } from './api';
import type { CampaignState } from '@/types';

/**
 * Owns the campaign projection for one session. The server is authoritative;
 * stale responses from a previous session are discarded before they land.
 */
export function useCampaignProjection(sessionId: string | null) {
  const [campaign, setCampaign] = useState<CampaignState | null>(null);
  const generation = useRef(0);

  const reload = useCallback(async (): Promise<CampaignState | null> => {
    const ticket = ++generation.current;
    if (!sessionId) return null;
    const next = await fetchCampaignState(sessionId);
    if (ticket !== generation.current) return next;
    setCampaign(next);
    return next;
  }, [sessionId]);

  useEffect(() => {
    void reload();
    return () => {
      generation.current += 1;
    };
  }, [reload]);

  return { campaign, setCampaign, reload };
}
