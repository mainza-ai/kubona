'use client';

import { fetchFleet, fetchRadar } from '@/lib/api';
import type { BackendInfo, RadarEndpoint } from '@/types';
import { ResourceRefusal } from './resource-store';
import { usePollingResource } from './use-polling-resource';

type FleetProjection = {
  backends: BackendInfo[];
  radarEndpoints: RadarEndpoint[];
  refused: string | null;
};

async function loadFleet(): Promise<FleetProjection> {
  const [fleet, radar] = await Promise.allSettled([fetchFleet(), fetchRadar()]);
  if (fleet.status === 'rejected' && radar.status === 'rejected') {
    throw fleet.reason instanceof Error ? fleet.reason : new Error(String(fleet.reason));
  }
  let backends: BackendInfo[] = [];
  let radarEndpoints: RadarEndpoint[] = [];
  let refused: string | null = null;
  if (fleet.status === 'fulfilled') {
    const data = fleet.value as { backends?: BackendInfo[]; refused?: boolean; refuse_reason?: string };
    if (data.refused) refused = data.refuse_reason || 'fleet refused';
    else backends = data.backends || [];
  } else {
    refused = fleet.reason instanceof Error ? fleet.reason.message : String(fleet.reason);
  }
  if (radar.status === 'fulfilled') {
    const data = radar.value as { radar_endpoints?: RadarEndpoint[]; refused?: boolean; refuse_reason?: string };
    if (data.refused) refused = data.refuse_reason || 'radar refused';
    else radarEndpoints = data.radar_endpoints || [];
  } else if (!refused) {
    refused = radar.reason instanceof Error ? radar.reason.message : String(radar.reason);
  }
  if (refused) throw new ResourceRefusal(refused, { backends, radarEndpoints, refused });
  return { backends, radarEndpoints, refused };
}

export function useFleetProjection() {
  const state = usePollingResource<FleetProjection>(loadFleet, {
    intervalMs: 1000,
    maxBackoffMs: 15_000,
  });
  return {
    ...state,
    fleet: state.data ?? { backends: [], radarEndpoints: [], refused: null },
  };
}
